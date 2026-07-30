# Physical drum P5: product integration

P5 exposes the double-headed physical Tom through the existing Tom voice
editor. The model selector is explicit: Algorithmic remains the default for a
fresh app and for every state blob written before the selector existed.
Switching models resets the current tail but preserves both parameter banks,
so the editor can A/B them without reinterpreting controls.

## Generated parameters

`internal/drum/params.go` is the single source for both procedural and physical
editor metadata. `cmd/gen-voiceparams` emits `PHYSICAL_TOM_PARAMS` beside the
existing voice tables. The physical bank has stable, append-only indices:

| Control | Model field or mapping                              | Range                    |
| ------- | --------------------------------------------------- | ------------------------ |
| SIZE    | shared batter/resonant diameter                     | 0.16–0.50 m              |
| B.TUNE  | `Batter.TensionNPerM`                               | 150–1400 N/m             |
| R.TUNE  | `Resonant.TensionNPerM`                             | 150–1400 N/m             |
| DAMP    | overall head loss scale                             | 0.25–4×                  |
| HIT.R   | `Strike.Radius01`                                   | 0–0.95                   |
| HIT.A   | `Strike.AngleRad`                                   | −180–180°                |
| HARD    | `Strike.Hardness01`                                 | 0–1                      |
| DEPTH   | `Cavity.DepthM`                                     | 0.05–0.60 m              |
| AIR     | `Cavity.Coupling01`                                 | 0–1                      |
| NLIN    | both Berger coefficients relative to their defaults | 0–2                      |
| MIC.R   | `Pickup.Radius01`                                   | 0–0.95                   |
| MIC.A   | `Pickup.AngleRad`                                   | −180–180°                |
| QUAL    | `Draft`, `Standard`, or `High`                      | 24, 48, or 96 modes/head |
| ASYM    | full cosine/sine pair-frequency separation          | 0–2 %                    |
| AXIS    | non-uniform-tension principal axis                  | −90–90°                  |
| D.TILT  | frequency-dependent loss terms, relative to DAMP    | 0–3×                     |

The normalized UI value is the command and persistence representation.
Engineering mappings, bounds, defaults, display units, accessible names, and
the discrete quality labels all come from the generated descriptor. The
physical adapter rebuilds a complete validated `PhysicalDrum` and atomically
reconfigures the model; a rejected change leaves the active configuration
untouched. Successful changes intentionally reset the current tail.

The AIR control uses the effective swept area
\(\widetilde A_i=gA_i\), with the same \(g\) in the cavity pressure drive and
the pressure force returned to each head. This makes zero an exact uncoupled
limit and retains the passive energy balance described in
[`physical-cavity.md`](physical-cavity.md).

## State and command boundary

The physical bank is separate from the procedural Tom bank throughout:

- Go exposes `SetPhysicalTomParam`, independent of `SetVoiceParam`;
- WASM and the Worker validate `setPhysicalTomParam` as a required method;
- queued edits use the same load/retry path as all other engine commands;
- the React editor selects the generated bank from the active model;
- RESET restores only the bank currently shown.

App-state format version 4 appended the original thirteen quantized physical
positions after the version-3 Tom selector. Version 5 appends ASYM and AXIS;
the original indices remain unchanged. Version 6 keeps the same width and
migrates only the exact former shipped HIT.R detent from the peripheral 0.45
radius to the corrected central 0.12 radius. User-edited hit positions remain
unchanged.

Versions 1 and 2 still decode without a selector and therefore choose
Algorithmic at the call site. Version 3 retains its stored model selection and
receives generated physical defaults. Version 4 restores its thirteen
positions and receives generated defaults for the two P6 controls. Reload and
share-link browser coverage exercises both the selected model and an edited
physical tuning control.

All controls use the existing keyboard-accessible slider implementation,
formatted live readouts, focus-safe modal dialog, reset announcement, and
disabled state while the engine is unavailable. Audition remains independent
of transport state.
