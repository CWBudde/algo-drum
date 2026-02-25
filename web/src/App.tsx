import { useEffect, useState } from "react";
import DrumMachine from "./components/DrumMachine";
import { loadWasm } from "./engine/wasmEngine";

export default function App() {
  const [wasmLoaded, setWasmLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadWasm()
      .then(() => setWasmLoaded(true))
      .catch((e: unknown) => setError(String(e)));
  }, []);

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        padding: "6px 16px",
        background: "#1A1008",
      }}
    >
      {error && (
        <p
          style={{
            color: "#ff6b6b",
            marginBottom: 12,
            fontFamily: "monospace",
            fontSize: 13,
          }}
        >
          Failed to load engine: {error}
        </p>
      )}
      {!wasmLoaded && !error && (
        <p
          style={{
            color: "#C4A07A",
            marginBottom: 16,
            fontFamily: "Georgia, serif",
          }}
        >
          Loading engine…
        </p>
      )}
      <DrumMachine wasmLoaded={wasmLoaded} />
    </div>
  );
}
