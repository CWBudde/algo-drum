import { encodeState } from "../algo/persistence";
import type { EngineState } from "../engine/engineState";

export interface ShareLocation {
  origin: string;
  pathname: string;
  search: string;
}

interface ShareHistory {
  replaceState(data: unknown, unused: string, url?: string | URL | null): void;
}

// buildShareUrl is deliberately pure: callers choose when the generated URL
// becomes browser history rather than hiding that mutation inside a getter.
export function buildShareUrl(
  state: EngineState,
  location: ShareLocation,
): string {
  return `${location.origin}${location.pathname}${location.search}#${encodeState(state)}`;
}

export function replaceShareUrl(history: ShareHistory, url: string): void {
  history.replaceState(null, "", url);
}
