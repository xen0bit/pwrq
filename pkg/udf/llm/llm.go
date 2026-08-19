package llm

import "github.com/itchyny/gojq"

// RegisterAll returns every LLM and agent cmdlet.
//
// These are network calls against a paid API, so like the censys family they
// are CLI-only: WebRegistry leaves them out, and the browser catalog reports
// them as unavailable rather than pretending a page could make them.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterInvokeLLM(),
		RegisterInvokeLLMRequest(),
		RegisterInvokeLLMBatch(),
		RegisterInvokeAgent(),
		RegisterInvokeAgentRequest(),
		RegisterInvokeEmbedding(),
		RegisterGetModel(),
		RegisterGetContext(),
		RegisterGetUsage(),
	}
}
