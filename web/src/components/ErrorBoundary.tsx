import { Component, type ErrorInfo, type ReactNode } from "react";
import "./ErrorBoundary.css";

interface Props {
  children: ReactNode;

  // Called once a crash has been caught, so the owner can shut down whatever
  // the unmounted subtree was driving (see App: the audio engine).
  onError?: (error: Error, info: ErrorInfo) => void;
}

interface State {
  error: Error | null;
}

// App-wide error boundary: catches render/lifecycle crashes anywhere below it
// so a bug in the drum machine shows a readable panel instead of a blank page.
// Styled to match the machine's skeuomorphic theme.
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Surface the component stack in the console for debugging; the UI keeps
    // the message human-readable.
    console.error("Unexpected UI error:", error, info.componentStack);

    this.props.onError?.(error, info);
  }

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div className="fault-panel" role="alert">
        <div className="fault-badge" aria-hidden="true">
          !
        </div>
        <h2 className="fault-title">Something went wrong</h2>
        <p className="fault-text">
          The drum machine hit an unexpected error and had to stop. Reloading
          the page usually clears it.
        </p>
        <pre className="fault-detail">{error.message}</pre>
        <button
          type="button"
          className="fault-btn"
          onClick={() => window.location.reload()}
        >
          Reload
        </button>
      </div>
    );
  }
}
