// Package censys exposes the Censys Platform API as cmdlets.
//
// The vocabulary follows Censys' own CLI, cencli: view an asset, enrich a host,
// search, aggregate, run CensEye, manage collections and tags, read the
// organization and its credits. Requests go through the vendor's Go SDK
// (github.com/censys/censys-sdk-go) rather than hand-rolled HTTP, so the URL
// shapes, the retry policy and the error bodies are Censys' own.
//
// Objects come back with the API's field names — snake_case, exactly as the
// Censys documentation and CenQL spell them — plus a PwrqType. Renaming
// `autonomous_system` to `AutonomousSystem` would invent a second vocabulary
// for a schema the user already knows from writing queries, so the cmdlets
// leave the payload alone. Cmdlet *parameters* are PowerShell-style and bind
// case-insensitively, like every other cmdlet here.
//
// One consequence of going through the SDK is worth knowing: the payload is
// the SDK's model of the response, not the bytes on the wire, so a field the
// pinned SDK version does not model is dropped before the pipeline sees it.
// New fields arrive by upgrading censys-sdk-go.
//
// Everything here reads, unless PWRQ_CENSYS_WRITE says otherwise. See
// WriteCmdlets.
package censys

import (
	"os"
	"strings"

	"github.com/itchyny/gojq"
)

// EnvWrite is what has to be set for the cmdlets that change something in a
// Censys organization to exist at all.
//
// pwrq's own name rather than one of the CENSYS_PLATFORM_* variables, because
// this is not a Censys setting: a token with write scope is a token with write
// scope whatever pwrq does, and what this governs is only whether this program
// offers a way to spend it.
const EnvWrite = "PWRQ_CENSYS_WRITE"

// WriteCmdlets are the cmdlets EnvWrite governs: everything whose request is a
// POST, a PATCH or a DELETE against the organization.
//
// The reason they are off by default is that a query is a thing people iterate
// on. A search that is wrong reads the wrong hosts and is rewritten; a
// `remove_censys_tag` that is wrong has already deleted a tag every asset in
// the organization was carrying, and no rerun brings it back. The read
// vocabulary is idempotent and these are not, so they are the ones that have
// to be asked for.
//
// new_censys_censeye_job is here for a slightly different reason than the rest.
// It edits nothing, but it creates a job in the organization and spends the
// organization's credits to do it, so it is a request that costs something
// somebody else can see. get_censys_censeye_job and get_censys_censeye_result
// still read the jobs it started, whoever started them.
var WriteCmdlets = []string{
	"new_censys_collection",
	"set_censys_collection",
	"remove_censys_collection",
	"new_censys_censeye_job",
	"new_censys_tag",
	"set_censys_tag",
	"remove_censys_tag",
	"add_censys_tag_assignment",
	"remove_censys_tag_assignment",
}

// WriteEnabled reports whether the write cmdlets are to be offered.
//
// Read at every call rather than once at start-up: the metadata catalogue and
// the registry both ask, and a value settled in an initializer would make the
// answer depend on which of them ran first.
func WriteEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvWrite))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// RegisterAll returns every Censys cmdlet that may be reached.
//
// Not "every Censys cmdlet": the write half is left unregistered unless
// EnvWrite says otherwise, so a query that names one fails to compile rather
// than reaching the API. That is the point of doing it here instead of
// refusing inside the cmdlet - a name that is not registered cannot be called
// by a typo, by an LLM writing a pipeline, or by a rule, and get_command does
// not list it either.
func RegisterAll() []gojq.CompilerOption {
	options := []gojq.CompilerOption{
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

		// Collections, reading only.
		RegisterGetCollection(),
		RegisterGetCollectionEvent(),

		// cencli's censeye, plus the threat list it draws on. Starting a job
		// is a write; reading the jobs and their pivots is not.
		RegisterGetCenseyeJob(),
		RegisterGetCenseyeResult(),
		RegisterGetThreat(),

		// cencli's tags, reading only.
		RegisterGetTag(),
		RegisterGetTagAssignment(),

		// cencli's org and credits.
		RegisterGetOrganization(),
		RegisterGetCredits(),
	}
	if WriteEnabled() {
		options = append(options, registerWrites()...)
	}
	return options
}

// registerWrites returns the cmdlets WriteCmdlets names, in the same order.
//
// Kept beside the list rather than folded into RegisterAll, so that the two
// can be checked against each other: a cmdlet added to one and forgotten in
// the other is a cmdlet that is either unreachable when it was asked for or
// reachable when it was not.
func registerWrites() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterNewCollection(),
		RegisterSetCollection(),
		RegisterRemoveCollection(),
		RegisterNewCenseyeJob(),
		RegisterNewTag(),
		RegisterSetTag(),
		RegisterRemoveTag(),
		RegisterAddTagAssignment(),
		RegisterRemoveTagAssignment(),
	}
}
