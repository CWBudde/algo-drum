import { useCallback, useEffect, useState } from "react";
import DrumMachine from "./components/DrumMachine";
import ErrorBoundary from "./components/ErrorBoundary";
import { loadWasm } from "./engine/wasmEngine";
import "./App.css";

type LoadState =
  | { status: "loading" }
  | { status: "ready" }
  | { status: "error"; message: string };

// Turn whatever the WASM loader throws into a short, human-readable line.
function describeError(e: unknown): string {
  if (e instanceof Error && e.message) return e.message;
  const text = String(e);
  return text === "[object Object]" ? "Unknown error" : text;
}

export default function App() {
  const [load, setLoad] = useState<LoadState>({ status: "loading" });

  const attemptLoad = useCallback(() => {
    setLoad({ status: "loading" });
    loadWasm()
      .then(() => setLoad({ status: "ready" }))
      .catch((e: unknown) =>
        setLoad({ status: "error", message: describeError(e) }),
      );
  }, []);

  useEffect(() => {
    attemptLoad();
  }, [attemptLoad]);

  return (
    <main className="app">
      {load.status === "error" && (
        <div className="app-fault" role="alert">
          <div className="app-fault-badge" aria-hidden="true">
            ⚠
          </div>
          <h2 className="app-fault-title">Audio engine failed to load</h2>
          <p className="app-fault-text">
            The drum machine couldn&rsquo;t start its WebAssembly audio engine.
            This usually means a network hiccup or a browser without WebAssembly
            support.
          </p>
          <pre className="app-fault-detail">{load.message}</pre>
          <button type="button" className="app-fault-btn" onClick={attemptLoad}>
            Retry
          </button>
        </div>
      )}
      {load.status === "loading" && (
        <p className="app-loading">Loading engine…</p>
      )}
      {load.status !== "error" && (
        <ErrorBoundary>
          <DrumMachine wasmLoaded={load.status === "ready"} />
        </ErrorBoundary>
      )}
    </main>
  );
}
