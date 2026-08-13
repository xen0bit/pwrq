package censys

import (
	"fmt"

	"github.com/censys/censys-sdk-go/models/components"
	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// defaultBuckets is how many buckets an aggregate asks for when the caller does
// not say. The API has no default of its own — number_of_buckets is a required
// field — so the cmdlet has to pick one.
const defaultBuckets = 50

// RegisterSearch registers search_censys, cencli's `search`.
//
// It emits one object per hit rather than the response envelope, so a query
// reads the way the rest of pwrq does: `[search_censys("...")] | length`
// counts what came back and `select` filters it. Paging is opt-in through
// {Pages: n}; the totals the envelope carries are what get_censys_aggregate is
// for.
func RegisterSearch() gojq.CompilerOption {
	const op = "search_censys"
	return common.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		query, rawOpts, err := identifier(op, "Query", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Fields       []string `param:"Fields"`
			PageSize     int      `param:"PageSize"`
			PageToken    string   `param:"PageToken"`
			Pages        int      `param:"Pages"`
			CollectionId string   `param:"CollectionId"`
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
			body := components.SearchQueryInputBody{
				Query:     query,
				Fields:    opts.Fields,
				PageSize:  optInt64(opts.PageSize),
				PageToken: optString(token),
			}

			// A collection scopes the same query to one collection's assets,
			// which is a different endpoint rather than a filter.
			var result *components.SearchQueryResponse
			if opts.CollectionId != "" {
				res, err := sdk.Collections.Search(ctx, operations.V3CollectionsSearchQueryRequest{
					CollectionUID:        opts.CollectionId,
					SearchQueryInputBody: body,
				})
				if err != nil {
					return nil, "", apiError(op, err)
				}
				if res.ResponseEnvelopeSearchQueryResponse != nil {
					result = res.ResponseEnvelopeSearchQueryResponse.Result
				}
			} else {
				res, err := sdk.GlobalData.Search(ctx, operations.V3GlobaldataSearchQueryRequest{
					SearchQueryInputBody: body,
				})
				if err != nil {
					return nil, "", apiError(op, err)
				}
				if res.ResponseEnvelopeSearchQueryResponse != nil {
					result = res.ResponseEnvelopeSearchQueryResponse.Result
				}
			}
			if result == nil {
				return nil, "", nil
			}

			batch, err := items(op, "Censys.Platform.Hit", result.Hits)
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

// RegisterGetAggregate registers get_censys_aggregate, cencli's `aggregate`.
//
// The whole point of an aggregate is the counts beside the buckets, so this one
// returns the result object rather than streaming the buckets out of it.
func RegisterGetAggregate() gojq.CompilerOption {
	const op = "get_censys_aggregate"
	return common.WithFunction(op, 1, 3, func(v any, args []any) any {
		// The field is always given explicitly, so the query is the half that
		// can come from the pipeline.
		query, field, rawOpts, err := pairArgs(op, "Query", "Field", 0, v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Buckets       int    `param:"Buckets"`
			FilterByQuery bool   `param:"FilterByQuery"`
			CountByLevel  string `param:"CountByLevel"`
			CollectionId  string `param:"CollectionId"`
		}
		if err := bindOptions(op, rest, &opts); err != nil {
			return err
		}
		if opts.Buckets < 0 {
			return fmt.Errorf("%s: Buckets must not be negative, got %d", op, opts.Buckets)
		}
		if opts.Buckets == 0 {
			opts.Buckets = defaultBuckets
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}

		body := components.SearchAggregateInputBody{
			Query:           query,
			Field:           field,
			NumberOfBuckets: int64(opts.Buckets),
			FilterByQuery:   &opts.FilterByQuery,
			CountByLevel:    optString(opts.CountByLevel),
		}

		var result *components.SearchAggregateResponse
		if opts.CollectionId != "" {
			res, err := sdk.Collections.Aggregate(ctx, operations.V3CollectionsSearchAggregateRequest{
				CollectionUID:            opts.CollectionId,
				SearchAggregateInputBody: body,
			})
			if err != nil {
				return apiError(op, err)
			}
			if res.ResponseEnvelopeSearchAggregateResponse != nil {
				result = res.ResponseEnvelopeSearchAggregateResponse.Result
			}
		} else {
			res, err := sdk.GlobalData.Aggregate(ctx, operations.V3GlobaldataSearchAggregateRequest{
				SearchAggregateInputBody: body,
			})
			if err != nil {
				return apiError(op, err)
			}
			if res.ResponseEnvelopeSearchAggregateResponse != nil {
				result = res.ResponseEnvelopeSearchAggregateResponse.Result
			}
		}

		obj, err := object(op, "Censys.Platform.Aggregate", result)
		if err != nil {
			return err
		}
		return obj
	})
}
