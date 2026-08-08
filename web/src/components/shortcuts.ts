export type GlobalShortcut =
  | { type: "playPause" }
  | { type: "stop" }
  | { type: "mutate" }
  | { type: "clear" }
  | { type: "muteTrack"; visualRow: number }
  | { type: "nextPreset" }
  | { type: "previousPreset" }
  | { type: "showHelp" };

export interface ShortcutKey {
  code: string;
  key: string;
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
  repeat: boolean;
}

// resolveGlobalShortcut deliberately uses physical key codes for commands and
// the reported character only for `?`. This keeps the transport/editing map
// stable across keyboard layouts while still letting the conventional help
// character work wherever that layout places it.
export function resolveGlobalShortcut(
  event: ShortcutKey,
): GlobalShortcut | null {
  if (event.repeat || event.altKey || event.ctrlKey || event.metaKey)
    return null;

  if (event.key === "?" || (event.code === "Slash" && event.shiftKey)) {
    return { type: "showHelp" };
  }

  if (event.code === "Backspace" && event.shiftKey) {
    return { type: "clear" };
  }

  if (event.code === "KeyP") {
    return { type: event.shiftKey ? "previousPreset" : "nextPreset" };
  }

  if (event.shiftKey) return null;

  switch (event.code) {
    case "Space":
      return { type: "playPause" };
    case "KeyS":
      return { type: "stop" };
    case "KeyM":
      return { type: "mutate" };
  }

  if (/^Digit[1-7]$/.test(event.code)) {
    return { type: "muteTrack", visualRow: Number(event.code.slice(-1)) - 1 };
  }

  return null;
}
