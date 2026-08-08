import { describe, expect, it } from "vitest";
import {
  resolveGlobalShortcut,
  type GlobalShortcut,
  type ShortcutKey,
} from "./shortcuts";

function key(code: string, overrides: Partial<ShortcutKey> = {}): ShortcutKey {
  return {
    code,
    key: code === "Space" ? " " : "",
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    shiftKey: false,
    repeat: false,
    ...overrides,
  };
}

describe("global shortcut map", () => {
  it.each<[ShortcutKey, GlobalShortcut]>([
    [key("Space"), { type: "playPause" }],
    [key("KeyS"), { type: "stop" }],
    [key("KeyM"), { type: "mutate" }],
    [key("Backspace", { shiftKey: true }), { type: "clear" }],
    [key("KeyP"), { type: "nextPreset" }],
    [key("KeyP", { shiftKey: true }), { type: "previousPreset" }],
    [key("Slash", { key: "?", shiftKey: true }), { type: "showHelp" }],
    [key("Digit7"), { type: "muteTrack", visualRow: 6 }],
  ])("maps %o", (event, expected) => {
    expect(resolveGlobalShortcut(event)).toEqual(expected);
  });

  it("ignores repeats, browser modifiers, shifted commands and unknown keys", () => {
    expect(resolveGlobalShortcut(key("KeyM", { repeat: true }))).toBeNull();
    expect(resolveGlobalShortcut(key("Digit1", { ctrlKey: true }))).toBeNull();
    expect(resolveGlobalShortcut(key("KeyS", { shiftKey: true }))).toBeNull();
    expect(resolveGlobalShortcut(key("Digit8"))).toBeNull();
  });
});
