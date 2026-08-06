import { describe, expect, it, vi } from "vitest";
import { decodeState } from "../algo/persistence";
import { createDefaultEngineState } from "../engine/engineState";
import { buildShareUrl, replaceShareUrl } from "./shareUrl";

describe("share URL action", () => {
  it("builds a URL without mutating its inputs", () => {
    const state = createDefaultEngineState();
    state.tempoBpm = 137;
    const before = state.pattern.slice();

    const url = buildShareUrl(state, {
      origin: "https://example.test",
      pathname: "/algo-drum/",
      search: "?theme=dark",
    });

    expect(url).toMatch(/^https:\/\/example\.test\/algo-drum\/\?theme=dark#/);
    expect(decodeState(url.slice(url.indexOf("#") + 1))?.tempoBpm).toBe(137);
    expect(state.pattern).toEqual(before);
  });

  it("updates history only when the explicit action is called", () => {
    const replaceState = vi.fn();
    const history = { replaceState };
    const url = "https://example.test/algo-drum/#state";

    expect(replaceState).not.toHaveBeenCalled();
    replaceShareUrl(history, url);

    expect(replaceState).toHaveBeenCalledExactlyOnceWith(null, "", url);
  });
});
