package poller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/adamdecaf/asic-rs-go/asicrs"
	"github.com/adamdecaf/hasherdash/internal/axetemp"
	"github.com/adamdecaf/hasherdash/internal/config"
	"github.com/adamdecaf/hasherdash/internal/models"
)

// AsicSource discovers and polls real miners via asic-rs-go.
type AsicSource struct {
	cfg         config.Config
	staticIPs   []string            // configured fixed IPs (always polled)
	discovered  map[string]struct{} // IPs found via subnet/range scan
	subnets     []string            // CIDRs to re-scan periodically
	ranges      []string            // range strings to re-scan periodically
	lastScan    time.Time           // zero forces a scan on the first collect
	forceRescan bool                // next collect runs a full subnet/range scan
	mu          sync.Mutex
}

// NewAsicSource builds a source from configured IPs / subnets / ranges.
func NewAsicSource(cfg config.Config) *AsicSource {
	return &AsicSource{
		cfg:        cfg,
		staticIPs:  append([]string(nil), cfg.MinerIPs...),
		discovered: make(map[string]struct{}),
		subnets:    append([]string(nil), cfg.MinerSubnets...),
		ranges:     append([]string(nil), cfg.MinerRanges...),
	}
}

func (a *AsicSource) Name() string { return "asic-rs" }

func (a *AsicSource) Collect(ctx context.Context) ([]models.Detail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ips, err := a.resolveIPs()
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no miner IPs configured (set ips/subnets/ranges in config file or MINER_IPS / MINER_SUBNET)")
	}

	type result struct {
		d   models.Detail
		err error
	}
	ch := make(chan result, len(ips))
	sem := make(chan struct{}, maxInt(a.cfg.Concurrent, 1))
	var wg sync.WaitGroup

	for _, ip := range ips {
		ip := ip
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				ch <- result{err: ctx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			d, err := pollOne(ip, a.cfg.ScanTimeoutSec)
			if err != nil {
				snap := models.Snapshot{
					IP:        ip,
					Error:     err.Error(),
					UpdatedAt: time.Now().UTC(),
				}
				models.ApplyStableID(&snap)
				ch <- result{d: models.Detail{Snapshot: snap}}
				return
			}
			ch <- result{d: d}
		}()
	}
	wg.Wait()
	close(ch)

	out := make([]models.Detail, 0, len(ips))
	var firstErr error
	for r := range ch {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		out = append(out, r.d)
	}
	// Partial success is OK; only fail hard if we got nothing usable.
	ok := 0
	for _, d := range out {
		if d.Error == "" {
			ok++
		}
	}
	if ok == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

// RequestRescan forces a full subnet/range discovery on the next Collect.
func (a *AsicSource) RequestRescan() {
	a.mu.Lock()
	a.forceRescan = true
	a.mu.Unlock()
}

// Forget drops discovered IPs (not static config) so pruned miners stop being polled
// until a future full scan finds them again. ids are miner IPs from store.Prune.
func (a *AsicSource) Forget(ids []string) {
	if len(ids) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	static := map[string]struct{}{}
	for _, ip := range a.staticIPs {
		static[ip] = struct{}{}
	}
	for _, ip := range ids {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if _, ok := static[ip]; ok {
			continue
		}
		delete(a.discovered, ip)
	}
}

func (a *AsicSource) resolveIPs() ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var scanErr error
	if a.shouldScanLocked() {
		scanErr = a.scanLocked()
		a.lastScan = time.Now()
	}

	seen := map[string]struct{}{}
	var ips []string
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if _, ok := seen[ip]; ok {
			return
		}
		seen[ip] = struct{}{}
		ips = append(ips, ip)
	}
	for _, ip := range a.staticIPs {
		add(ip)
	}
	for ip := range a.discovered {
		add(ip)
	}

	if len(ips) == 0 && scanErr != nil {
		return nil, scanErr
	}
	return ips, nil
}

func (a *AsicSource) shouldScanLocked() bool {
	if len(a.subnets) == 0 && len(a.ranges) == 0 {
		return false
	}
	if a.forceRescan {
		a.forceRescan = false
		return true
	}
	// Always scan on the first collect.
	if a.lastScan.IsZero() {
		return true
	}
	// RescanInterval <= 0: startup scan only.
	if a.cfg.RescanInterval <= 0 {
		return false
	}
	return time.Since(a.lastScan) >= a.cfg.RescanInterval
}

func (a *AsicSource) scanLocked() error {
	var scanErr error
	for _, subnet := range a.subnets {
		f, err := asicrs.NewFactoryFromSubnet(subnet)
		if err != nil {
			scanErr = fmt.Errorf("subnet %s: %w", subnet, err)
			log.Printf("asic-rs: %v", scanErr)
			continue
		}
		configureFactory(f, a.cfg)
		log.Printf("asic-rs: scanning subnet %s (%d hosts)", subnet, f.Len())
		miners, err := f.Scan()
		if err != nil {
			f.Close()
			scanErr = fmt.Errorf("subnet %s: %w", subnet, err)
			log.Printf("asic-rs: %v", scanErr)
			continue
		}
		for _, m := range miners {
			if ip, err := m.IP(); err == nil {
				ip = strings.TrimSpace(ip)
				if ip != "" {
					a.discovered[ip] = struct{}{}
				}
			}
			m.Close()
		}
		f.Close()
	}
	for _, rng := range a.ranges {
		f, err := asicrs.NewFactoryFromRange(rng)
		if err != nil {
			scanErr = fmt.Errorf("range %s: %w", rng, err)
			log.Printf("asic-rs: %v", scanErr)
			continue
		}
		configureFactory(f, a.cfg)
		log.Printf("asic-rs: scanning range %s (%d hosts)", rng, f.Len())
		miners, err := f.Scan()
		if err != nil {
			f.Close()
			scanErr = fmt.Errorf("range %s: %w", rng, err)
			log.Printf("asic-rs: %v", scanErr)
			continue
		}
		for _, m := range miners {
			if ip, err := m.IP(); err == nil {
				ip = strings.TrimSpace(ip)
				if ip != "" {
					a.discovered[ip] = struct{}{}
				}
			}
			m.Close()
		}
		f.Close()
	}
	log.Printf("asic-rs: discovery complete — %d static + %d discovered IPs",
		len(a.staticIPs), len(a.discovered))
	return scanErr
}

func configureFactory(f *asicrs.Factory, cfg config.Config) {
	f.SetPortCheck(true)
	f.SetIdentificationTimeoutSecs(uint64(maxInt(cfg.ScanTimeoutSec, 3)))
	f.SetConcurrentLimit(cfg.Concurrent)
	f.SetAdaptiveConcurrency()
}

func pollOne(ip string, timeoutSec int) (models.Detail, error) {
	factory := asicrs.NewFactory()
	defer factory.Close()
	factory.SetIdentificationTimeoutSecs(uint64(maxInt(timeoutSec, 3)))
	factory.SetPortCheck(true)

	miner, err := factory.GetMiner(ip)
	if err != nil {
		return models.Detail{}, err
	}
	defer miner.Close()

	data, err := miner.GetData()
	if err != nil {
		return models.Detail{}, err
	}
	return detailFromMinerData(data), nil
}

func detailFromMinerData(data *asicrs.MinerData) models.Detail {
	now := time.Now().UTC()
	snap := models.Snapshot{
		IP:         data.IP,
		Make:       data.DeviceInfo.Make,
		Model:      data.DeviceInfo.Model,
		Firmware:   data.DeviceInfo.Firmware,
		Algo:       data.DeviceInfo.Algo,
		IsMining:   data.IsMining,
		HashrateTH: data.HashrateTH(),
		UpdatedAt:  now,
		LastSeen:   now,
	}
	if data.MAC != nil {
		snap.MAC = *data.MAC
	}
	if data.Hostname != nil {
		snap.Hostname = *data.Hostname
	}
	if data.FirmwareVersion != nil {
		snap.FirmwareVer = *data.FirmwareVersion
	}
	if data.SerialNumber != nil {
		snap.Serial = *data.SerialNumber
	}
	models.ApplyStableID(&snap)
	if data.ExpectedHashrate != nil {
		snap.ExpectedTH = data.ExpectedHashrate.TH()
	}
	if data.Wattage != nil {
		snap.Wattage = *data.Wattage
	}
	if data.Efficiency != nil {
		snap.Efficiency = *data.Efficiency
	}
	if data.AverageTemperature != nil {
		snap.AvgTempC = *data.AverageTemperature
	}
	if data.FluidTemperature != nil {
		snap.FluidTempC = *data.FluidTemperature
	}
	if data.TotalChips != nil {
		snap.TotalChips = int(*data.TotalChips)
	}
	if data.ExpectedChips != nil {
		snap.ExpectedChips = int(*data.ExpectedChips)
	}
	if data.ExpectedHashboards != nil {
		snap.Boards = int(*data.ExpectedHashboards)
	} else if n, ok := data.DeviceInfo.Hardware.BoardCount(); ok {
		snap.Boards = n
	}
	if data.ExpectedFans != nil {
		snap.Fans = int(*data.ExpectedFans)
	}
	if data.Uptime != nil {
		snap.UptimeSec = int64(data.Uptime.Duration().Seconds())
	}
	if data.LightFlashing != nil {
		snap.LightFlashing = *data.LightFlashing
	}
	for _, m := range data.Messages {
		snap.Messages = append(snap.Messages, m.Message)
	}

	detail := models.Detail{Snapshot: snap}
	var asicTemps, vrTemps []float64
	for _, b := range data.Hashboards {
		row := models.BoardRow{Position: int(b.Position)}
		if b.Hashrate != nil {
			row.HashrateTH = b.Hashrate.TH()
		}

		// Board/PCB temperature. On Bitaxe/Nerdaxe this is the VR sensor (vrTemp).
		var boardVR *float64
		if b.BoardTemperature != nil {
			row.BoardTempC = *b.BoardTemperature
			row.VRTempC = *b.BoardTemperature
			row.HasVRTemp = true
			vrTemps = append(vrTemps, *b.BoardTemperature)
			boardVR = b.BoardTemperature
		}

		// ASIC temps come from real chip sensors only.
		//
		// asic-rs 0.7.2 Bitaxe/Nerdaxe backends incorrectly copy vrTemp into
		// inlet_chip_temperature / outlet_chip_temperature. Treat those as
		// chip-chain sensors only when they differ from board/VR temp.
		var boardASIC []float64
		for _, c := range b.Chips {
			if c.Temperature != nil {
				boardASIC = append(boardASIC, *c.Temperature)
			}
		}
		if b.InletChipTemperature != nil && !axetemp.SameTemp(b.InletChipTemperature, boardVR) {
			row.ASICTempIn = *b.InletChipTemperature
			row.HasASICIn = true
			boardASIC = append(boardASIC, *b.InletChipTemperature)
		}
		if b.OutletChipTemperature != nil && !axetemp.SameTemp(b.OutletChipTemperature, boardVR) {
			row.ASICTempOut = *b.OutletChipTemperature
			row.HasASICOut = true
			boardASIC = append(boardASIC, *b.OutletChipTemperature)
		}
		if minV, maxV, ok := axetemp.MinMax(boardASIC); ok {
			asicTemps = append(asicTemps, boardASIC...)
			// Derive per-board in/out from chip readings when chain sensors
			// were discarded as VR duplicates (common on single-chip axes).
			if !row.HasASICIn {
				row.ASICTempIn = minV
				row.HasASICIn = true
			}
			if !row.HasASICOut {
				row.ASICTempOut = maxV
				row.HasASICOut = true
			}
		}

		if b.WorkingChips != nil {
			row.WorkingChips = int(*b.WorkingChips)
		}
		if b.ExpectedChips != nil {
			row.ExpectedChips = int(*b.ExpectedChips)
		}
		if b.Frequency != nil {
			row.Frequency = *b.Frequency
		}
		if b.Voltage != nil {
			row.Voltage = *b.Voltage
		}
		row.Active = b.Active
		row.Tuned = b.Tuned
		detail.Hashboards = append(detail.Hashboards, row)
	}
	if minV, maxV, ok := axetemp.MinMax(asicTemps); ok {
		snap.HasASICTemp = true
		snap.ASICTempMin = minV
		snap.ASICTempMax = maxV
	}
	if minV, maxV, ok := axetemp.MinMax(vrTemps); ok {
		snap.HasVRTemp = true
		snap.VRTempMin = minV
		snap.VRTempMax = maxV
	}

	// Bitaxe / Nerdaxe: asic-rs often drops the real chip "temp" field and only
	// surfaces vrTemp (as board + fake inlet/outlet). Fetch /api/system/info
	// when we still lack a distinct ASIC reading.
	if axetemp.NeedsFallback(snap) {
		if chip, vr, ok := axetemp.FetchSystemTemps(snap.IP); ok {
			if !snap.HasASICTemp {
				snap.HasASICTemp = true
				snap.ASICTempMin = chip
				snap.ASICTempMax = chip
				if len(detail.Hashboards) > 0 {
					detail.Hashboards[0].ASICTempIn = chip
					detail.Hashboards[0].ASICTempOut = chip
					detail.Hashboards[0].HasASICIn = true
					detail.Hashboards[0].HasASICOut = true
				}
			}
			if !snap.HasVRTemp && vr != nil {
				snap.HasVRTemp = true
				snap.VRTempMin = *vr
				snap.VRTempMax = *vr
				if len(detail.Hashboards) > 0 {
					detail.Hashboards[0].VRTempC = *vr
					detail.Hashboards[0].BoardTempC = *vr
					detail.Hashboards[0].HasVRTemp = true
				}
			}
		}
	}

	// Headline average: prefer real ASIC max over asic-rs average, which for
	// Bitaxe is computed from board/VR temps only.
	switch {
	case snap.HasASICTemp:
		snap.AvgTempC = snap.ASICTempMax
	case snap.AvgTempC == 0 && snap.HasVRTemp:
		snap.AvgTempC = snap.VRTempMax
	}

	detail.Snapshot = snap
	for _, f := range data.Fans {
		if f.RPM != nil {
			detail.FanRPMs = append(detail.FanRPMs, *f.RPM)
		}
	}
	for _, g := range data.Pools {
		for _, p := range g.Pools {
			row := models.PoolRow{Group: g.Name, Active: p.Active, Alive: p.Alive}
			if p.URL != nil {
				row.URL = p.URL.String()
				snap.PoolHosts = appendUnique(snap.PoolHosts, p.URL.Host)
			}
			if p.User != nil {
				row.User = *p.User
				snap.PoolUsers = appendUnique(snap.PoolUsers, *p.User)
			}
			if p.AcceptedShares != nil {
				row.Accepted = *p.AcceptedShares
			}
			if p.RejectedShares != nil {
				row.Rejected = *p.RejectedShares
			}
			detail.Pools = append(detail.Pools, row)
		}
	}
	detail.Snapshot = snap
	return detail
}

func appendUnique(ss []string, v string) []string {
	if v == "" {
		return ss
	}
	for _, s := range ss {
		if s == v {
			return ss
		}
	}
	return append(ss, v)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
