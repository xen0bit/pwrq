package censys

import (
	"fmt"
	"io"
	"time"

	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/itchyny/gojq"
)

// timelineWindow is how far back a timeline reaches when the caller does not
// say. The API demands both ends of the range, and a week is the span the
// event history is usually read over.
const timelineWindow = 7 * 24 * time.Hour

// RegisterGetHost registers get_censys_host, cencli's `view host`.
func RegisterGetHost() gojq.CompilerOption {
	const op = "get_censys_host"
	return gojq.WithFunction(op, 0, 2, func(v any, args []any) any {
		id, rawOpts, err := identifier(op, "HostId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			AtTime string `param:"AtTime"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}
		atTime, err := parseTime(op, "AtTime", opts.AtTime)
		if err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.GlobalData.GetHost(ctx, operations.V3GlobaldataAssetHostRequest{
			HostID: id,
			AtTime: atTime,
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeHostAsset == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Host", res.ResponseEnvelopeHostAsset.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterGetCertificate registers get_censys_certificate, cencli's
// `view certificate`. {Raw: true} returns the PEM text instead of the parsed
// asset, which is what you need to hand the certificate to another tool.
func RegisterGetCertificate() gojq.CompilerOption {
	const op = "get_censys_certificate"
	return gojq.WithFunction(op, 0, 2, func(v any, args []any) any {
		id, rawOpts, err := identifier(op, "CertificateId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Raw bool `param:"Raw"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}

		if opts.Raw {
			res, err := sdk.GlobalData.GetCertificateRaw(ctx, operations.V3GlobaldataAssetCertificateRawRequest{
				CertificateID: id,
			})
			if err != nil {
				return apiError(op, err)
			}
			if res.ResponseStream == nil {
				return fmt.Errorf("%s: the API returned an empty response", op)
			}
			defer func() { _ = res.ResponseStream.Close() }()
			pem, err := io.ReadAll(res.ResponseStream)
			if err != nil {
				return fmt.Errorf("%s: reading the PEM stream: %w", op, err)
			}
			return string(pem)
		}

		res, err := sdk.GlobalData.GetCertificate(ctx, operations.V3GlobaldataAssetCertificateRequest{
			CertificateID: id,
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeCertificateAsset == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Certificate", res.ResponseEnvelopeCertificateAsset.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterGetWebProperty registers get_censys_webproperty, cencli's
// `view web-property`. The identifier is hostname:port.
func RegisterGetWebProperty() gojq.CompilerOption {
	const op = "get_censys_webproperty"
	return gojq.WithFunction(op, 0, 2, func(v any, args []any) any {
		id, rawOpts, err := identifier(op, "WebPropertyId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			AtTime string `param:"AtTime"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}
		atTime, err := parseTime(op, "AtTime", opts.AtTime)
		if err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.GlobalData.GetWebProperty(ctx, operations.V3GlobaldataAssetWebpropertyRequest{
			WebpropertyID: id,
			AtTime:        atTime,
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeWebpropertyAsset == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.WebProperty", res.ResponseEnvelopeWebpropertyAsset.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterGetEnrichment registers get_censys_enrichment, cencli's `enrich`:
// the lightweight host lookup meant for high-volume automation.
func RegisterGetEnrichment() gojq.CompilerOption {
	const op = "get_censys_enrichment"
	return gojq.WithFunction(op, 0, 2, func(v any, args []any) any {
		id, rawOpts, err := identifier(op, "HostIp", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)
		if err := bindOptions(op, rest, &struct{}{}); err != nil {
			return err
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.GlobalData.GetHostEnrichment(ctx, operations.V3GlobaldataAssetHostEnrichmentRequest{
			HostIP: id,
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeHostEnrichmentAsset == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.HostEnrichment", res.ResponseEnvelopeHostEnrichmentAsset.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// timelineOptions are the ends of an event-history window.
//
// The names are the API's, and so is the surprise in them: StartTime is the
// end of the range nearest to now and EndTime the one furthest away. Renaming
// them to something less confusing would only mean the Censys documentation no
// longer described the parameters, so they keep their names and the inversion
// is documented instead.
type timelineOptions struct {
	StartTime string `param:"StartTime"`
	EndTime   string `param:"EndTime"`
}

// window resolves the pair, defaulting to the last timelineWindow.
func (o timelineOptions) window(op string) (start, end time.Time, err error) {
	startPtr, err := parseTime(op, "StartTime", o.StartTime)
	if err != nil {
		return start, end, err
	}
	endPtr, err := parseTime(op, "EndTime", o.EndTime)
	if err != nil {
		return start, end, err
	}
	start = time.Now().UTC()
	if startPtr != nil {
		start = *startPtr
	}
	end = start.Add(-timelineWindow)
	if endPtr != nil {
		end = *endPtr
	}

	// Given how the two are named, writing them the wrong way round is the
	// mistake to expect. Catching it here says which one is which; the API
	// would only say the range was invalid.
	if !end.Before(start) {
		return start, end, fmt.Errorf("%s: EndTime must be older than StartTime — StartTime is the end of the range nearest to now — got StartTime %s and EndTime %s",
			op, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	return start, end, nil
}

// RegisterGetHostTimeline registers get_censys_host_timeline, cencli's
// `history host`. It emits one object per event.
func RegisterGetHostTimeline() gojq.CompilerOption {
	const op = "get_censys_host_timeline"
	return gojq.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		id, rawOpts, err := identifier(op, "HostId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts timelineOptions
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}
		start, end, err := opts.window(op)
		if err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}
		res, err := sdk.GlobalData.GetHostTimeline(ctx, operations.V3GlobaldataAssetHostTimelineRequest{
			HostID:    id,
			StartTime: start,
			EndTime:   end,
		})
		if err != nil {
			return gojq.NewIter(apiError(op, err))
		}
		if res.ResponseEnvelopeHostTimeline == nil || res.ResponseEnvelopeHostTimeline.Result == nil {
			return gojq.NewIter[any]()
		}
		events, err := items(op, "Censys.Platform.HostEvent", res.ResponseEnvelopeHostTimeline.Result.Events)
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(events...)
	})
}

// RegisterGetWebPropertyTimeline registers get_censys_webproperty_timeline,
// cencli's `history web-property`.
func RegisterGetWebPropertyTimeline() gojq.CompilerOption {
	const op = "get_censys_webproperty_timeline"
	return gojq.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		id, rawOpts, err := identifier(op, "WebPropertyId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts timelineOptions
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}
		start, end, err := opts.window(op)
		if err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}
		res, err := sdk.GlobalData.GetWebPropertyTimeline(ctx, operations.V3GlobaldataAssetWebpropertyTimelineRequest{
			WebpropertyID: id,
			StartTime:     start,
			EndTime:       end,
		})
		if err != nil {
			return gojq.NewIter(apiError(op, err))
		}
		if res.ResponseEnvelopeWebpropertyTimeline == nil || res.ResponseEnvelopeWebpropertyTimeline.Result == nil {
			return gojq.NewIter[any]()
		}
		events, err := items(op, "Censys.Platform.WebPropertyEvent", res.ResponseEnvelopeWebpropertyTimeline.Result.Events)
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(events...)
	})
}

// RegisterGetHostService registers get_censys_host_service: the service
// observation history behind cencli's host view, one range per output.
func RegisterGetHostService() gojq.CompilerOption {
	const op = "get_censys_host_service"
	return gojq.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		id, rawOpts, err := identifier(op, "HostId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			StartTime         string `param:"StartTime"`
			EndTime           string `param:"EndTime"`
			Port              int    `param:"Port"`
			Protocol          string `param:"Protocol"`
			TransportProtocol string `param:"TransportProtocol"`
			PageSize          int    `param:"PageSize"`
			PageToken         string `param:"PageToken"`
			Pages             int    `param:"Pages"`
		}
		// Unset means a single page. {Pages: 0} follows the cursor to the
		// end, which is opt-in because every page costs credits.
		opts.Pages = 1
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}
		if _, err := parseTime(op, "StartTime", opts.StartTime); err != nil {
			return gojq.NewIter(err)
		}
		if _, err := parseTime(op, "EndTime", opts.EndTime); err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}

		var transport *operations.TransportProtocol
		if opts.TransportProtocol != "" {
			t := operations.TransportProtocol(opts.TransportProtocol)
			transport = &t
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.GlobalData.ListServicesOnHost(ctx, operations.V3GlobaldataServiceOnHostRequest{
				HostID:            id,
				StartTime:         optString(opts.StartTime),
				EndTime:           optString(opts.EndTime),
				Port:              optInt(opts.Port),
				Protocol:          optString(opts.Protocol),
				TransportProtocol: transport,
				PageSize:          optInt(opts.PageSize),
				PageToken:         optString(token),
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeServicesOnHostResponse == nil || res.ResponseEnvelopeServicesOnHostResponse.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeServicesOnHostResponse.Result
			batch, err := items(op, "Censys.Platform.ServiceObservation", result.Ranges)
			if err != nil {
				return nil, "", err
			}
			return batch, result.NextPageToken, nil
		})
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}
