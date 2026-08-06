package drum

// ProtocolVersion identifies the semantics of the AlgoDrum JS API that
// cmd/wasm registers. web/src/engine/audioWorker.ts pins the same number and
// refuses to run against a mismatching engine, because algo_drum.wasm is an
// unhashed build artifact that caches independently of the hashed JS bundle:
// without this, a skewed pair loads happily and plays wrong.
//
// Bump it whenever an existing entry point changes meaning rather than
// existing: argument order or count, the unit or range of an argument, the
// shape of EngineState, or what a call does. Purely *adding* a method needs no
// bump — the worker's REQUIRED_METHODS already rejects an engine that lacks
// one.
//
// This is unrelated to persistence.ts's FORMAT_VERSION, which versions saved
// patterns rather than the live API.
const ProtocolVersion = 1
