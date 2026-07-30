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

| Control | Model field or mapping                              | Range                       |
| ------- | --------------------------------------------------- | --------------------------- |
| SIZE    | shared batter/resonant diameter                     | 0.16–0.50 m                 |
| B.TUNE  | `Batter.TensionNPerM`, via `RetuneTension`          | 300–3500 N/m                |
| R.TUNE  | `Resonant.TensionNPerM`, via `RetuneTension`        | 300–3500 N/m                |
| DAMP    | overall head loss scale                             | 0.25–4×                     |
| HIT.R   | `Strike.Radius01`                                   | 0–0.95                      |
| HIT.A   | `Strike.AngleRad`                                   | −180–180°                   |
| HARD    | `Strike.Hardness01`                                 | 0–1                         |
| DEPTH   | `Cavity.DepthM`                                     | 0.05–0.60 m                 |
| AIR     | `Cavity.Coupling01`                                 | 0–1                         |
| NLIN    | both Berger coefficients relative to their defaults | 0–2                         |
| MIC.R   | `Pickup.Radius01`                                   | 0–0.95                      |
| MIC.A   | `Pickup.AngleRad`                                   | −180–180°                   |
| QUAL    | `Draft`, `Standard`, or `High`                      | 48, 96, or 160 batter modes |
| ASYM    | full cosine/sine pair-frequency separation          | 0–2 %                       |
| AXIS    | non-uniform-tension principal axis                  | −90–90°                     |
| D.TILT  | frequency-dependent loss terms, relative to DAMP    | 0–3×                        |
| ATK.L   | `Attack.LevelRelative`                              | 0–0.15                      |
| ATK.T   | `Attack.CentreHz`                                   | 500 Hz–8 kHz                |

B.TUNE and R.TUNE go through `RetuneTension` rather than writing the tension
field directly. The loss coefficients in the configuration are quoted at its
tension, so a bare assignment leaves ζ — and with it the whole decay calibration
— drifting with the tuning knob, which it used to do by a factor of three across
the range. See [`physical-calibration.md`](physical-calibration.md#tuning-and-constant-q).

AIR spans zero coupling up to the calibrated air spring, not up to the rigid
enclosure: `Cavity.StiffnessScale` is fitted in the configuration and is not
exposed as a control, so the knob's top of travel is now the measured 10–20 %
(0,1) split rather than the 1.87 ratio the rigid formula produces. See
[`physical-cavity.md`](physical-cavity.md).

QUAL is the _batter_ head's oscillator budget. The resonant head runs the same
selection and then keeps only the axisymmetric modes that anything can excite, so
it costs 6 oscillators at Standard rather than a second full bank — which is what
paid for the tiers doubling. See [`physical-hybrid.md`](physical-hybrid.md).

MIC.R is now the model's strongest timbral control rather than a near-field
weighting mistake. It sets the microphone's observation direction, and with it
how much non-axisymmetric content reaches it: on axis, only the axisymmetric
modes radiate at all. ATK.L and ATK.T balance and place the stochastic high band
that covers what modal synthesis cannot reach.

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
radius to the central 0.12 radius. User-edited hit positions remain unchanged.

Versions 7 and 8 add the second Tom and Percussion tracks and migrate the
reordered mixer strips; version 9 appends Tom 2's own model choice and parameter
bank; version 10 widens both physical banks for D.TILT; version 11 keeps those
bytes and moves the HIT.R detent again, off the 0.12 centre to 0.30, in **both**
physical banks; version 12 widens both banks for ATK.L and ATK.T; version 13
keeps version 12's bytes and rescales ATK.L, whose range narrowed from 0–0.3 to
0–0.15 when the attack layer became three bands.

ATK.L's rescale has the same two-rule shape as the HIT.R detent: a position still
sitting on version 12's default moves to version 13's default, and every other
position doubles. Both Tom banks get it.

The two HIT.R rules stay separately gated, at `version < 6` and `version < 11`.
Merging them would overwrite a version-6-or-later blob that a user had
deliberately dragged back to 0.45. The version-11 rule applies to Tom 2's bank as
well as Tom 1's, which the version-6 rule did not need to — that bank did not
exist before version 9, so it could never hold a pre-version-6 detent.

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
