import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App.jsx";
import "./styles.css";

// Clear storage on window load during tests
if (__DEV__) {
  window.addEventListener("load", () => {
    try { localStorage.clear(); } catch {}
  });
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
