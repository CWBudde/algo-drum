package drum

// ProtocolVersion identifies the semantics of the AlgoDrum JS API and the
// worker/worklet transport snapshots built on top of it. audioWorker.ts pins
// the same number and refuses to run against a mismatching engine, while
// wasmEngine.ts uses it to cache-bust the independently cached worklet.
//
// Bump it whenever an existing entry point changes meaning rather than
// existing: argument order or count, the unit or range of an argument, the
// shape of EngineState, or what a call does. Purely *adding* a method needs no
// bump — the worker's REQUIRED_METHODS already rejects an engine that lacks
// one.
//
// This is unrelated to persistence.ts's FORMAT_VERSION, which versions saved
// patterns rather than the live API.
const ProtocolVersion = 5
