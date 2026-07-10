import { useEffect, useState } from "react";
import DrumMachine from "./components/DrumMachine";
import { loadWasm } from "./engine/wasmEngine";
import "./App.css";

export default function App() {
  const [wasmLoaded, setWasmLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadWasm()
      .then(() => setWasmLoaded(true))
      .catch((e: unknown) => setError(String(e)));
  }, []);

  return (
    <main className="app">
      {error && (
        <p className="app-error" role="alert">
          Failed to load engine: {error}
        </p>
      )}
      {!wasmLoaded && !error && <p className="app-loading">Loading engine…</p>}
      <DrumMachine wasmLoaded={wasmLoaded} />
    </main>
  );
}
