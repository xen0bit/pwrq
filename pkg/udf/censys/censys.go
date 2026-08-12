// Package censys exposes the Censys Platform API as cmdlets.
//
// The vocabulary follows Censys' own CLI, cencli: view an asset, enrich a host,
// search, aggregate, run CensEye, manage collections and tags, read the
// organization and its credits. Requests go through the vendor's Go SDK
// (github.com/censys/censys-sdk-go) rather than hand-rolled HTTP, so the URL
// shapes, the retry policy and the error bodies are Censys' own.
//
// Objects come back with the API's field names — snake_case, exactly as the
// Censys documentation and CenQL spell them — plus a PSTypeName. Renaming
// `autonomous_system` to `AutonomousSystem` would invent a second vocabulary
// for a schema the user already knows from writing queries, so the cmdlets
// leave the payload alone. Cmdlet *parameters* are PowerShell-style and bind
// case-insensitively, like every other cmdlet here.
//
// One consequence of going through the SDK is worth knowing: the payload is
// the SDK's model of the response, not the bytes on the wire, so a field the
// pinned SDK version does not model is dropped before the pipeline sees it.
// New fields arrive by upgrading censys-sdk-go.
package censys

import "github.com/itchyny/gojq"

// RegisterAll returns every Censys cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterGetContext(),

		// Assets: cencli's view, enrich and history.
		RegisterGetHost(),
		RegisterGetCertificate(),
		RegisterGetWebProperty(),
		RegisterGetEnrichment(),
		RegisterGetHostTimeline(),
		RegisterGetWebPropertyTimeline(),
		RegisterGetHostService(),

		// cencli's search and aggregate, globally or within a collection.
		RegisterSearch(),
		RegisterGetAggregate(),

		// Collections.
		RegisterGetCollection(),
		RegisterNewCollection(),
		RegisterSetCollection(),
		RegisterRemoveCollection(),
		RegisterGetCollectionEvent(),

		// cencli's censeye, plus the threat list it draws on.
		RegisterNewCenseyeJob(),
		RegisterGetCenseyeJob(),
		RegisterGetCenseyeResult(),
		RegisterGetThreat(),

		// cencli's tags.
		RegisterGetTag(),
		RegisterNewTag(),
		RegisterSetTag(),
		RegisterRemoveTag(),
		RegisterGetTagAssignment(),
		RegisterAddTagAssignment(),
		RegisterRemoveTagAssignment(),

		// cencli's org and credits.
		RegisterGetOrganization(),
		RegisterGetCredits(),
	}
}
