import { jsxs as _jsxs, jsx as _jsx } from "react/jsx-runtime";
import { useEffect, useState } from "react";
import DrumMachine from "./components/DrumMachine";
import { loadWasm } from "./engine/wasmEngine";
export default function App() {
    const [wasmLoaded, setWasmLoaded] = useState(false);
    const [error, setError] = useState(null);
    useEffect(() => {
        loadWasm()
            .then(() => setWasmLoaded(true))
            .catch((e) => setError(String(e)));
    }, []);
    return (_jsxs("div", { style: {
            minHeight: "100vh",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            padding: "6px 16px",
            background: "#0A0B0D",
        }, children: [error && (_jsxs("p", { style: {
                    color: "#ff6b6b",
                    marginBottom: 12,
                    fontFamily: "monospace",
                    fontSize: 13,
                }, children: ["Failed to load engine: ", error] })), !wasmLoaded && !error && (_jsx("p", { style: {
                    color: "#C4A07A",
                    marginBottom: 16,
                    fontFamily: "Georgia, serif",
                }, children: "Loading engine\u2026" })), _jsx(DrumMachine, { wasmLoaded: wasmLoaded })] }));
}
