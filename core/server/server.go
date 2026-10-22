//go:build !darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"nekobox_core/gen"
	"nekobox_core/internal"
	"nekobox_core/internal/boxbox"
	"nekobox_core/internal/boxmain"
	"nekobox_core/internal/process"
	"nekobox_core/internal/wg"

	//	"nekobox_core/internal/sys"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/shlex"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/experimental/clashapi"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/service"
)

// server is used to implement myservice.MyServiceServer.
type server struct {
	profileMu sync.Mutex
}

// To returns a pointer to the given value.
func To[T any](v T) *T {
	return &v
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		return false
	}
	return !info.IsDir()
}

func (s *server) IsSupported(ctx context.Context, in *gen.Type) (*gen.Supported, error) {
	out := new(gen.Supported)
	ok := true
	if in.Type == "naive" {
		ok = CheckNaive()
	}
	out.Ok = ok
	return out, nil
}

func (s *server) CacheHTTP(ctx context.Context, in *gen.CacheURLRequest) (*gen.CacheURLResult_, error) {
	out := new(gen.CacheURLResult_)
	var exists bool
	out.FilePath = internal.CacheHttpBool(in.HTTPURL, in.UseDefaultOutbound, &exists)
	out.Exists = exists
	return out, nil
}

func (s *server) Start(ctx context.Context, in *gen.LoadConfigReq) (*gen.ErrorResp, error) {
	out := new(gen.ErrorResp)
	if !s.profileMu.TryLock() {
		out.Error = "Another profile operation is in progress"
		return out, nil
	}
	defer s.profileMu.Unlock()
	var err error

	defer func() {
		if err != nil {
			out.Error = (err.Error())
		}
	}()

	if internal.Debug {
		log.Println("Start:", in.CoreConfig)
	}

	if internal.CurrentBox() != nil {
		err = errors.New("previous profile is running or still closing; stop it before starting another profile")
		return out, nil
	}

	if in.NeedExtraProcess {
		args, e := shlex.Split(in.GetExtraProcessArgs_())
		if e != nil {
			err = E.Cause(e, "Failed to parse args")
			return out, nil
		}
		if in.ExtraProcessConf != "" {
			extraConfPath := in.ExtraProcessConfDir + string(os.PathSeparator) + "extra.conf"
			f, e := os.OpenFile(extraConfPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0700)
			if e != nil {
				err = E.Cause(e, "Failed to open extra.conf")
				return out, nil
			}
			_, e = f.WriteString(in.ExtraProcessConf)
			if e != nil {
				err = E.Cause(e, "Failed to write extra.conf")
				return out, nil
			}
			_ = f.Close()
			for idx, arg := range args {
				if strings.Contains(arg, "%s") {
					args[idx] = fmt.Sprintf(arg, extraConfPath)
					break
				}
			}
		}

		internal.ExtraProcess = process.NewProcess(in.ExtraProcessPath, args, in.ExtraNoOut)
		err = internal.ExtraProcess.Start()
		if err != nil {
			internal.ExtraProcess = nil
			return out, nil
		}
	}

	instance, cancel, createErr := boxmain.Create([]byte(in.CoreConfig))
	err = createErr
	if err != nil {
		if internal.ExtraProcess != nil {
			internal.ExtraProcess.Stop()
			internal.ExtraProcess = nil
		}
		return out, nil
	}
	internal.SetInstance(instance, cancel)
	//	if strings.Contains(
	//		in.CoreConfig, "tun-in") && strings.Contains(
	//		in.CoreConfig, "172.19.0.1/24") {
	//		err := sys.SetSystemDNS("172.19.0.2", boxInstance.Network().InterfaceMonitor())
	//		if err != nil {
	//			log.Println("Failed to set system DNS:", err)
	//		}
	//		needUnsetDNS = true
	//	}

	return out, nil
}

func (s *server) EnableSystemProxy(ctx context.Context, in *gen.SystemProxy) (*gen.ErrorResp, error) {
	out := new(gen.ErrorResp)
	var err error

	defer func() {
		if err != nil {
			out.Error = (err.Error())
		}
	}()

	err = internal.SetSystemProxy(ctx, in.Address, uint16(in.Port), in.SupportSocks)

	return out, nil
}

func (s *server) DisableSystemProxy(ctx context.Context, in *gen.EmptyReq) (*gen.ErrorResp, error) {
	out := new(gen.ErrorResp)
	var err error

	defer func() {
		if err != nil {
			out.Error = (err.Error())
		}
	}()

	err = internal.ResetSystemProxy()

	return out, nil
}

func (s *server) Stop(ctx context.Context, in *gen.EmptyReq) (*gen.ErrorResp, error) {
	out := new(gen.ErrorResp)
	if !s.profileMu.TryLock() {
		out.Error = "Another profile operation is in progress"
		return out, nil
	}
	defer s.profileMu.Unlock()
	var err error

	internal.ResetSystemProxy()

	defer func() {
		if err != nil {
			out.Error = (err.Error())
		}
	}()

	instance, cancel := internal.CurrentInstance()
	if instance == nil {
		return out, err
	}

	//	if needUnsetDNS {
	//		needUnsetDNS = false
	//		err := sys.SetSystemDNS("Empty", boxInstance.Network().InterfaceMonitor())
	//		if err != nil {
	//			log.Println("Failed to unset system DNS:", err)
	//		}
	//	}
	done := instance.BeginClose(cancel, log.Println)
	err = instance.CloseWithTimeout(cancel, time.Second*6, log.Println)
	select {
	case <-done:
		internal.ClearInstance(instance)
	default:
		// Do not permit another TUN instance while the old one still owns
		// the adapter. Clear only this generation once it really finishes.
		go func() { <-done; internal.ClearInstance(instance) }()
	}

	if internal.ExtraProcess != nil {
		internal.ExtraProcess.Stop()
		internal.ExtraProcess = nil
	}

	return out, nil
}

func (s *server) CheckConfig(ctx context.Context, in *gen.LoadConfigReq) (*gen.ErrorResp, error) {
	out := new(gen.ErrorResp)
	err := boxmain.Check([]byte(in.CoreConfig))
	if err != nil {
		out.Error = (err.Error())
		return out, nil
	}
	return out, nil
}

func (s *server) QueryIPTest(ctx context.Context, in *gen.EmptyReq) (out *gen.QueryIPTestResponse, _ error) {
	results := IPReporter.Results()
	out = &gen.QueryIPTestResponse{}
	for _, r := range results {
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}

		out.Results = append(out.Results, &gen.IPTestResp{
			OutboundTag: (r.Tag),
			IP:          (r.Result.IP),
			CountryCode: (r.Result.CountryCode),
			Error:       (errStr),
		})
	}
	return out, nil
}

func (s *server) IPTest(ctx context.Context, in *gen.IPTestRequest) (*gen.QueryIPTestResponse, error) {
	testCtx := currentTestContext()
	out := new(gen.QueryIPTestResponse)
	var testInstance *boxbox.Box
	var cancel context.CancelFunc
	var err error
	testInstance, cancel, err = boxmain.Create([]byte(in.Config))
	if err != nil {
		return nil, err
	}
	defer testInstance.CloseWithTimeout(cancel, 2*time.Second, log.Println)

	outboundTags := in.OutboundTags
	if in.UseDefaultOutbound {
		outbound := testInstance.Outbound().Default()
		outboundTags = []string{outbound.Tag()}
	}

	maxConcurrency := in.MaxConcurrency
	if maxConcurrency >= MaxConcurrentTests || maxConcurrency == 0 {
		maxConcurrency = MaxConcurrentTests
	}
	timeout := time.Duration(in.TestTimeoutMs) * time.Millisecond
	results := BatchIPTest(testCtx, testInstance, outboundTags, int(maxConcurrency), timeout)

	res := make([]*gen.IPTestResp, 0, len(results))
	for idx, data := range results {
		errStr := ""
		if data.Error != nil {
			errStr = data.Error.Error()
		}
		tag := outboundTags[idx]

		res = append(res, &gen.IPTestResp{
			OutboundTag: (tag),
			IP:          (data.Result.IP),
			CountryCode: (data.Result.CountryCode),
			Error:       (errStr),
		})
	}
	out.Results = res
	return out, nil
}

// prefer the requested tags, the default one follows route.final
func resolveLiveTags(i *boxbox.Box, tags []string) []string {
	outbounds := service.FromContext[adapter.OutboundManager](i.Context())
	known := make([]string, 0, len(tags))
	for _, tag := range tags {
		if _, found := outbounds.Outbound(tag); found {
			known = append(known, tag)
		}
	}
	if len(known) == 0 {
		return []string{i.Outbound().Default().Tag()}
	}
	return known
}

func (s *server) Test(ctx context.Context, in *gen.TestReq) (*gen.TestResp, error) {
	testCtx := currentTestContext()
	out := new(gen.TestResp)
	var testInstance *boxbox.Box
	var cancel context.CancelFunc
	var err error
	var twice = true
	if in.TestCurrent {
		testInstance = internal.CurrentBox()
		if testInstance == nil {
			out.Results = []*gen.URLTestResp{{
				OutboundTag: ("proxy"),
				LatencyMs:   (int32(0)),
				Error:       ("Instance is not running"),
			}}
			return out, nil
		}
	} else {
		testInstance, cancel, err = boxmain.Create([]byte(in.Config))
		if err != nil {
			return out, err
		}
		defer testInstance.CloseWithTimeout(cancel, 2*time.Second, log.Println)
	}

	outboundTags := in.OutboundTags
	if in.TestCurrent {
		outboundTags = resolveLiveTags(testInstance, in.OutboundTags)
	} else if in.UseDefaultOutbound {
		outbound := testInstance.Outbound().Default()
		outboundTags = []string{outbound.Tag()}
	}

	var maxConcurrency = in.MaxConcurrency
	if maxConcurrency >= 500 || maxConcurrency == 0 {
		maxConcurrency = MaxConcurrentTests
	}
	results := BatchURLTest(testCtx, testInstance, outboundTags, in.URL,
		int(maxConcurrency), twice, time.Duration(in.TestTimeoutMs)*time.Millisecond)

	res := make([]*gen.URLTestResp, 0)
	for idx, data := range results {
		errStr := ""
		if data.Error != nil {
			errStr = data.Error.Error()
		}
		res = append(res, &gen.URLTestResp{
			OutboundTag: (outboundTags[idx]),
			LatencyMs:   (int32(data.Duration.Milliseconds())),
			Error:       (errStr),
		})
	}

	out.Results = res
	return out, nil
}

func (s *server) StopTest(ctx context.Context, in *gen.EmptyReq) (*gen.EmptyResp, error) {
	out := new(gen.EmptyResp)

	resetTestContext()

	return out, nil
}

func (s *server) QueryURLTest(ctx context.Context, in *gen.EmptyReq) (*gen.QueryURLTestResponse, error) {
	out := new(gen.QueryURLTestResponse)
	results := URLReporter.Results()
	for _, r := range results {
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}
		out.Results = append(out.Results, &gen.URLTestResp{
			OutboundTag: (r.Tag),
			LatencyMs:   (int32(r.Duration.Milliseconds())),
			Error:       (errStr),
		})
	}
	return out, nil
}

func (s *server) QueryStats(ctx context.Context, in *gen.EmptyReq) (*gen.QueryStatsResp, error) {
	out := new(gen.QueryStatsResp)
	out.Ups = make(map[string]int64)
	out.Downs = make(map[string]int64)
	if instance := internal.CurrentBox(); instance != nil {
		clash := service.FromContext[adapter.ClashServer](instance.Context())
		if clash != nil {
			cApi, ok := clash.(*clashapi.Server)
			if !ok {
				log.Println("Failed to assert clash server")
				return out, E.New("invalid clash server type")
			}
			outbounds := service.FromContext[adapter.OutboundManager](instance.Context())
			if outbounds == nil {
				log.Println("Failed to get outbound manager")
				return out, E.New("nil outbound manager")
			}
			endpoints := service.FromContext[adapter.EndpointManager](instance.Context())
			if endpoints == nil {
				log.Println("Failed to get endpoint manager")
				return out, E.New("nil endpoint manager")
			}
			for _, ob := range outbounds.Outbounds() {
				u, d := cApi.TrafficManager().TotalOutbound(ob.Tag())
				out.Ups[ob.Tag()] = u
				out.Downs[ob.Tag()] = d
			}
			for _, ep := range endpoints.Endpoints() {
				u, d := cApi.TrafficManager().TotalOutbound(ep.Tag())
				out.Ups[ep.Tag()] = u
				out.Downs[ep.Tag()] = d
			}
		}
	}
	return out, nil
}

func (s *server) ListConnections(ctx context.Context, in *gen.EmptyReq) (*gen.ListConnectionsResp, error) {
	out := new(gen.ListConnectionsResp)
	instance := internal.CurrentBox()
	if instance == nil {
		return out, nil
	}
	if service.FromContext[adapter.ClashServer](instance.Context()) == nil {
		return out, errors.New("no clash server found")
	}
	clash, ok := service.FromContext[adapter.ClashServer](instance.Context()).(*clashapi.Server)
	if !ok {
		return out, errors.New("invalid state, should not be here")
	}
	connections := clash.TrafficManager().Connections()

	res := make([]*gen.ConnectionMetaData, 0)
	for _, c := range connections {
		process := ""
		if c.Metadata.ProcessInfo != nil {
			spl := strings.Split(c.Metadata.ProcessInfo.ProcessPath, string(os.PathSeparator))
			process = spl[len(spl)-1]
		}
		r := &gen.ConnectionMetaData{
			ID:        (c.ID.String()),
			CreatedAt: (c.CreatedAt.UnixMilli()),
			Upload:    (c.Upload.Load()),
			Download:  (c.Download.Load()),
			Outbound:  (c.Outbound),
			Network:   (c.Metadata.Network),
			Dest:      (c.Metadata.Destination.String()),
			Protocol:  (c.Metadata.Protocol),
			Domain:    (c.Metadata.Domain),
			Process:   (process),
		}
		res = append(res, r)
	}
	out.Connections = res
	return out, nil
}

func (s *server) SpeedTest(ctx context.Context, in *gen.SpeedTestRequest) (*gen.SpeedTestResponse, error) {
	testCtx := currentTestContext()
	out := new(gen.SpeedTestResponse)
	if !in.TestDownload && !in.TestUpload && !in.SimpleDownload && !in.OnlyCountry {
		return out, errors.New("cannot run empty test")
	}
	var testInstance *boxbox.Box
	var cancel context.CancelFunc
	outboundTags := in.OutboundTags
	var err error
	if in.TestCurrent {
		testInstance = internal.CurrentBox()
		if testInstance == nil {
			out.Results = []*gen.SpeedTestResult_{{
				OutboundTag: ("proxy"),
				Error:       ("Instance is not running"),
			}}
			return out, nil
		}
	} else {
		testInstance, cancel, err = boxmain.Create([]byte(in.Config))
		if err != nil {
			return out, err
		}
		defer testInstance.CloseWithTimeout(cancel, 2*time.Second, log.Println)
	}

	if in.TestCurrent {
		outboundTags = resolveLiveTags(testInstance, in.OutboundTags)
	} else if in.UseDefaultOutbound {
		outbound := testInstance.Outbound().Default()
		outboundTags = []string{outbound.Tag()}
	}

	results := BatchSpeedTest(testCtx, testInstance, outboundTags,
		in.TestDownload, in.TestUpload, in.SimpleDownload,
		in.SimpleDownloadAddr, time.Duration(in.TimeoutMs)*time.Millisecond,
		in.OnlyCountry, in.CountryConcurrency)

	res := make([]*gen.SpeedTestResult_, 0)
	for _, data := range results {
		errStr := ""
		if data.Error != nil {
			errStr = data.Error.Error()
		}
		res = append(res, &gen.SpeedTestResult_{
			DlSpeed:       (data.DlSpeed),
			UlSpeed:       (data.UlSpeed),
			Latency:       (data.Latency),
			OutboundTag:   (data.Tag),
			Error:         (errStr),
			ServerName:    (data.ServerName),
			ServerCountry: (data.ServerCountry),
			Cancelled:     (data.Cancelled),
		})
	}

	out.Results = res
	return out, nil
}

func (s *server) QuerySpeedTest(ctx context.Context, in *gen.EmptyReq) (*gen.QuerySpeedTestResponse, error) {
	out := new(gen.QuerySpeedTestResponse)
	res, isRunning := SpTQuerier.Result()
	errStr := ""
	if res.Error != nil {
		errStr = res.Error.Error()
	}
	out.Result_ = &gen.SpeedTestResult_{
		DlSpeed:       (res.DlSpeed),
		UlSpeed:       (res.UlSpeed),
		Latency:       (res.Latency),
		OutboundTag:   (res.Tag),
		Error:         (errStr),
		ServerName:    (res.ServerName),
		ServerCountry: (res.ServerCountry),
		Cancelled:     (res.Cancelled),
	}
	out.IsRunning = (isRunning)
	return out, nil
}

func (s *server) GenWgKeyPair(ctx context.Context, _ *gen.EmptyReq) (out *gen.GenWgKeyPairResponse, _ error) {
	res := new(gen.GenWgKeyPairResponse)
	privateKey, err := wg.GeneratePrivateKey()
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	res.PrivateKey = privateKey.String()
	res.PublicKey = privateKey.PublicKey().String()
	return res, nil
}

func (s *server) QueryCountryTest(ctx context.Context, in *gen.EmptyReq) (*gen.QueryCountryTestResponse, error) {
	out := new(gen.QueryCountryTestResponse)
	results := CountryResults.Results()
	for _, res := range results {
		var errStr string
		if res.Error != nil {
			errStr = res.Error.Error()
		}
		out.Results = append(out.Results, &gen.SpeedTestResult_{
			DlSpeed:       (res.DlSpeed),
			UlSpeed:       (res.UlSpeed),
			Latency:       (res.Latency),
			OutboundTag:   (res.Tag),
			Error:         (errStr),
			ServerName:    (res.ServerName),
			ServerCountry: (res.ServerCountry),
			Cancelled:     (res.Cancelled),
		})
	}
	return out, nil
}
