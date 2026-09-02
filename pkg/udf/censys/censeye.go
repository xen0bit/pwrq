package censys

import (
	"fmt"
	"strings"

	"github.com/censys/censys-sdk-go/models/components"
	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterNewCenseyeJob registers new_censys_censeye_job, the first half of
// cencli's `censeye`.
//
// CensEye is asynchronous: this starts the pivot analysis and returns the job,
// and get_censys_censeye_result reads what it found once its status says so.
// Blocking here until the job finished would hide a long-running scan inside
// what looks like an ordinary jq expression.
//
// The target defaults to a host; {Type: "webproperty"} or {Type:
// "certificate"} analyses the other two asset kinds.
//
// A write: registered only when EnvWrite asks for it.
func RegisterNewCenseyeJob() gojq.CompilerOption {
	const op = "new_censys_censeye_job"
	return common.WithFunction(op, 0, 2, func(v any, args []any) any {
		id, rawOpts, err := identifier(op, "AssetId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Type string `param:"Type"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}

		target := components.CenseyeTarget{}
		switch strings.ToLower(opts.Type) {
		case "", "host":
			target.HostID = &id
		case "webproperty", "web_property", "web-property":
			target.WebpropertyID = &id
		case "certificate", "cert":
			target.CertificateID = &id
		default:
			return fmt.Errorf("%s: Type must be host, webproperty or certificate, got %q", op, opts.Type)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.ThreatHunting.CreateCenseyeJob(ctx, operations.V3ThreathuntingCenseyeJobsCreateRequest{
			CreateCenseyeJobInputBody: components.CreateCenseyeJobInputBody{Target: target},
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeCenseyeJob == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.CenseyeJob", res.ResponseEnvelopeCenseyeJob.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterGetCenseyeJob registers get_censys_censeye_job: one job by ID, or
// the list of them when no ID is given.
func RegisterGetCenseyeJob() gojq.CompilerOption {
	const op = "get_censys_censeye_job"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		jobID, rawOpts, err := optionalIdentifier(op, "JobId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			HostId        string `param:"HostId"`
			WebPropertyId string `param:"WebPropertyId"`
			CertificateId string `param:"CertificateId"`
			PageSize      int    `param:"PageSize"`
			PageToken     string `param:"PageToken"`
			Pages         int    `param:"Pages"`
		}
		// Unset means a single page. {Pages: 0} follows the cursor to the
		// end, which is opt-in because every page costs credits.
		opts.Pages = 1
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}

		if jobID != "" {
			res, err := sdk.ThreatHunting.GetCenseyeJob(ctx, operations.V3ThreathuntingCenseyeJobsGetRequest{JobID: jobID})
			if err != nil {
				return gojq.NewIter(apiError(op, err))
			}
			if res.ResponseEnvelopeCenseyeJob == nil {
				return gojq.NewIter(fmt.Errorf("%s: the API returned an empty response", op))
			}
			obj, err := object(op, "Censys.Platform.CenseyeJob", res.ResponseEnvelopeCenseyeJob.Result)
			if err != nil {
				return gojq.NewIter(err)
			}
			return gojq.NewIter[any](obj)
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.ThreatHunting.ListCenseyeJobs(ctx, operations.V3ThreathuntingCenseyeJobsListRequest{
				HostID:        optString(opts.HostId),
				WebpropertyID: optString(opts.WebPropertyId),
				CertificateID: optString(opts.CertificateId),
				PageSize:      optInt(opts.PageSize),
				PageToken:     optString(token),
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeCenseyeJobsListResponse == nil || res.ResponseEnvelopeCenseyeJobsListResponse.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeCenseyeJobsListResponse.Result
			batch, err := items(op, "Censys.Platform.CenseyeJob", result.Jobs)
			if err != nil {
				return nil, "", err
			}
			return batch, deref(result.NextPageToken), nil
		})
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}

// RegisterGetCenseyeResult registers get_censys_censeye_result: the pivots a
// finished CensEye job found, one per output.
func RegisterGetCenseyeResult() gojq.CompilerOption {
	const op = "get_censys_censeye_result"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		jobID, rawOpts, err := identifier(op, "JobId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			PageSize  int    `param:"PageSize"`
			PageToken string `param:"PageToken"`
			Pages     int    `param:"Pages"`
		}
		// Unset means a single page. {Pages: 0} follows the cursor to the
		// end, which is opt-in because every page costs credits.
		opts.Pages = 1
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.ThreatHunting.GetCenseyeJobResults(ctx, operations.V3ThreathuntingCenseyeJobResultsRequest{
				JobID:     jobID,
				PageSize:  optInt(opts.PageSize),
				PageToken: optString(token),
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeCenseyeResultsResponse == nil || res.ResponseEnvelopeCenseyeResultsResponse.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeCenseyeResultsResponse.Result
			batch, err := items(op, "Censys.Platform.CenseyeResult", result.Results)
			if err != nil {
				return nil, "", err
			}
			return batch, deref(result.NextPageToken), nil
		})
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}

// RegisterGetThreat registers get_censys_threat: the threats Censys is
// tracking, optionally narrowed by a CenQL query. The endpoint returns the
// whole list in one response, so there is nothing to page through.
func RegisterGetThreat() gojq.CompilerOption {
	const op = "get_censys_threat"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		query, rawOpts, err := optionalIdentifier(op, "Query", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)
		if err := bindOptions(op, rest, &struct{}{}); err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}
		res, err := sdk.ThreatHunting.ListThreats(ctx, operations.V3ThreathuntingThreatsListRequest{
			Query: optString(query),
		})
		if err != nil {
			return gojq.NewIter(apiError(op, err))
		}
		if res.ResponseEnvelopeThreatsListResponse == nil || res.ResponseEnvelopeThreatsListResponse.Result == nil {
			return gojq.NewIter[any]()
		}
		threats, err := items(op, "Censys.Platform.Threat", res.ResponseEnvelopeThreatsListResponse.Result.Threats)
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(threats...)
	})
}
