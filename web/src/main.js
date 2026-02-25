import { jsx as _jsx } from "react/jsx-runtime";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
createRoot(document.getElementById("root")).render(_jsx(StrictMode, { children: _jsx(App, {}) }));
if (import.meta.env.PROD && "serviceWorker" in navigator) {
    window.addEventListener("load", () => {
        const baseUrl = import.meta.env.BASE_URL;
        const swUrl = `${baseUrl}sw.js`;
        void navigator.serviceWorker.register(swUrl, { scope: baseUrl });
    });
}
