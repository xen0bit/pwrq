package censys

import (
	"fmt"

	"github.com/censys/censys-sdk-go/models/components"
	"github.com/censys/censys-sdk-go/models/operations"
	"github.com/itchyny/gojq"
)

// RegisterGetCollection registers get_censys_collection.
//
// With a UID it fetches that collection; without one it lists them, one object
// per collection, which is how you find the UID that search_censys's
// CollectionId option wants.
func RegisterGetCollection() gojq.CompilerOption {
	const op = "get_censys_collection"
	return gojq.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		uid, rawOpts, err := optionalIdentifier(op, "CollectionId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			Status    []string `param:"Status"`
			PageSize  int      `param:"PageSize"`
			PageToken string   `param:"PageToken"`
			Pages     int      `param:"Pages"`
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

		if uid != "" {
			res, err := sdk.Collections.Get(ctx, operations.V3CollectionsCrudGetRequest{CollectionUID: uid})
			if err != nil {
				return gojq.NewIter(apiError(op, err))
			}
			if res.ResponseEnvelopeCollection == nil {
				return gojq.NewIter(fmt.Errorf("%s: the API returned an empty response", op))
			}
			obj, err := object(op, "Censys.Platform.Collection", res.ResponseEnvelopeCollection.Result)
			if err != nil {
				return gojq.NewIter(err)
			}
			return gojq.NewIter[any](obj)
		}

		statuses := make([]operations.CollectionStatuses, 0, len(opts.Status))
		for _, s := range opts.Status {
			statuses = append(statuses, operations.CollectionStatuses(s))
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.Collections.List(ctx, operations.V3CollectionsCrudListRequest{
				PageSize:           optInt64(opts.PageSize),
				PageToken:          optString(token),
				CollectionStatuses: statuses,
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeListCollectionsResponseV1 == nil || res.ResponseEnvelopeListCollectionsResponseV1.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeListCollectionsResponseV1.Result
			batch, err := items(op, "Censys.Platform.Collection", result.Collections)
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

// collectionBody is the writable shape of a collection.
type collectionBody struct {
	Name        string `param:"Name"`
	Query       string `param:"Query"`
	Description string `param:"Description"`
}

// RegisterNewCollection registers new_censys_collection.
func RegisterNewCollection() gojq.CompilerOption {
	const op = "new_censys_collection"
	return gojq.WithFunction(op, 0, 1, func(v any, args []any) any {
		props, err := propertyArgs(op, v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(props)

		var body collectionBody
		if err := bindOptions(op, rest, &body); err != nil {
			return err
		}
		if body.Name == "" || body.Query == "" {
			return fmt.Errorf("%s: Name and Query are required", op)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.Collections.Create(ctx, operations.V3CollectionsCrudCreateRequest{
			CrudCreateInputBody: &components.CrudCreateInputBody{
				Name:        body.Name,
				Query:       body.Query,
				Description: optString(body.Description),
			},
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeCollection == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Collection", res.ResponseEnvelopeCollection.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterSetCollection registers set_censys_collection.
//
// The endpoint is a whole-object replace rather than a patch: Name and Query
// are both required, and omitting Description clears it.
func RegisterSetCollection() gojq.CompilerOption {
	const op = "set_censys_collection"
	return gojq.WithFunction(op, 1, 2, func(v any, args []any) any {
		uid, props, err := identifier(op, "CollectionId", v, args)
		if err != nil {
			return err
		}
		connOpts, rest := splitOptions(props)

		var body collectionBody
		if err := bindOptions(op, rest, &body); err != nil {
			return err
		}
		if body.Name == "" || body.Query == "" {
			return fmt.Errorf("%s: Name and Query are required; the API replaces the collection rather than patching it", op)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return err
		}
		res, err := sdk.Collections.Update(ctx, operations.V3CollectionsCrudUpdateRequest{
			CollectionUID: uid,
			CrudUpdateInputBody: &components.CrudUpdateInputBody{
				Name:        body.Name,
				Query:       body.Query,
				Description: optString(body.Description),
			},
		})
		if err != nil {
			return apiError(op, err)
		}
		if res.ResponseEnvelopeCollection == nil {
			return fmt.Errorf("%s: the API returned an empty response", op)
		}
		obj, err := object(op, "Censys.Platform.Collection", res.ResponseEnvelopeCollection.Result)
		if err != nil {
			return err
		}
		return obj
	})
}

// RegisterRemoveCollection registers remove_censys_collection. Like the other
// removers here it returns the UID it deleted, so the deletion is visible in a
// pipeline instead of vanishing into a null.
func RegisterRemoveCollection() gojq.CompilerOption {
	const op = "remove_censys_collection"
	return gojq.WithFunction(op, 0, 2, func(v any, args []any) any {
		uid, rawOpts, err := identifier(op, "CollectionId", v, args)
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
		if _, err := sdk.Collections.Delete(ctx, operations.V3CollectionsCrudDeleteRequest{CollectionUID: uid}); err != nil {
			return apiError(op, err)
		}
		return uid
	})
}

// RegisterGetCollectionEvent registers get_censys_collection_event: what
// changed in a collection, one object per event.
func RegisterGetCollectionEvent() gojq.CompilerOption {
	const op = "get_censys_collection_event"
	return gojq.WithIterFunction(op, 0, 2, func(v any, args []any) gojq.Iter {
		uid, rawOpts, err := identifier(op, "CollectionId", v, args)
		if err != nil {
			return gojq.NewIter(err)
		}
		connOpts, rest := splitOptions(rawOpts)

		var opts struct {
			ChangeType       []string `param:"ChangeType"`
			AssetChangeType  []string `param:"AssetChangeType"`
			StatusChangeType []string `param:"StatusChangeType"`
			StartTime        string   `param:"StartTime"`
			EndTime          string   `param:"EndTime"`
			PageSize         int      `param:"PageSize"`
			PageToken        string   `param:"PageToken"`
			Pages            int      `param:"Pages"`
		}
		// Unset means a single page. {Pages: 0} follows the cursor to the
		// end, which is opt-in because every page costs credits.
		opts.Pages = 1
		if err := bindOptions(op, rest, &opts); err != nil {
			return gojq.NewIter(err)
		}
		startTime, err := parseTime(op, "StartTime", opts.StartTime)
		if err != nil {
			return gojq.NewIter(err)
		}
		endTime, err := parseTime(op, "EndTime", opts.EndTime)
		if err != nil {
			return gojq.NewIter(err)
		}

		sdk, _, ctx, err := call(op, connOpts)
		if err != nil {
			return gojq.NewIter(err)
		}

		changeTypes := make([]operations.ChangeTypes, 0, len(opts.ChangeType))
		for _, s := range opts.ChangeType {
			changeTypes = append(changeTypes, operations.ChangeTypes(s))
		}
		assetChangeTypes := make([]operations.AssetChangeTypes, 0, len(opts.AssetChangeType))
		for _, s := range opts.AssetChangeType {
			assetChangeTypes = append(assetChangeTypes, operations.AssetChangeTypes(s))
		}
		statusChangeTypes := make([]operations.StatusChangeTypes, 0, len(opts.StatusChangeType))
		for _, s := range opts.StatusChangeType {
			statusChangeTypes = append(statusChangeTypes, operations.StatusChangeTypes(s))
		}

		results, err := walkPages(op, opts.PageToken, opts.Pages, func(token string) ([]any, string, error) {
			res, err := sdk.Collections.ListEvents(ctx, operations.V3CollectionsListEventsRequest{
				CollectionUID:     uid,
				PageSize:          optInt(opts.PageSize),
				PageToken:         optString(token),
				ChangeTypes:       changeTypes,
				AssetChangeTypes:  assetChangeTypes,
				StatusChangeTypes: statusChangeTypes,
				StartTime:         startTime,
				EndTime:           endTime,
			})
			if err != nil {
				return nil, "", apiError(op, err)
			}
			if res.ResponseEnvelopeCollectionEventsResponse == nil || res.ResponseEnvelopeCollectionEventsResponse.Result == nil {
				return nil, "", nil
			}
			result := res.ResponseEnvelopeCollectionEventsResponse.Result
			batch, err := items(op, "Censys.Platform.CollectionEvent", result.Events)
			if err != nil {
				return nil, "", err
			}
			return batch, result.NextPage, nil
		})
		if err != nil {
			return gojq.NewIter(err)
		}
		return gojq.NewIter(results...)
	})
}
