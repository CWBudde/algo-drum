# Physical tom sound audit

Audit date: 2026-07-29

The first integrated physical Tom was mechanically stable but did not sound
like a tom. This audit rechecked every signal stage and found three audible
modeling errors rather than a cosmetic EQ problem.

## Findings

| Stage                | Audit result                                                                                                                                                                                                                                                                                                                                                                |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Circular modes       | Fourier–Bessel frequencies, pair normalization, strike projection, ordering, and exact damped state transitions remain analytically and numerically covered.                                                                                                                                                                                                                |
| Geometry and heads   | The 12-inch head diameter, film surface densities, 104/112 Hz uncoupled head tuning, and 8-inch shell depth are plausible reduced-model values. They remain explicitly adjustable and are not labelled as a commercial-drum fit.                                                                                                                                            |
| Strike position      | The old default radius was 0.45. Measurements describe the ordinary tom playing spot as central; peripheral hits intentionally emphasize weaker high modes. The default moved to 0.12, and later to 0.30: a hit that close to the centre excites the axisymmetric modes and very little else, which is a tuned thump rather than a tom. See PLAN.md S5.                     |
| Stick contact        | HARD 0.7 produced only 34 samples at 48 kHz, or 0.71 ms. Measured 12-inch-tom contact is about 8 ms at quiet level and 5.5 ms at loud level. Contact now follows those velocity endpoints and hardness scales around them.                                                                                                                                                  |
| Head/cavity dynamics | The passive rank-one cavity solve, zero-coupling limit, energy balance, coupled-mode split, and independent frequency-domain comparison remain valid.                                                                                                                                                                                                                       |
| Observation          | The old microphone sum added both heads at equal distance, phase, orientation, and polarity. Under corrected contact, this nearly cancelled the 108 Hz coupled fundamental. The pickup is now explicitly batter-side; resonant radiation remains separate while its cavity feedback stays fully audible through the batter head.                                            |
| Nonlinearity         | The discrete-gradient Berger update remains passive. With corrected excitation, the isolated loud first mode moves about 158.7→150.1 Hz (96.9 cents) and the overtone centroid rises about 360→367 Hz from quiet to loud.                                                                                                                                                   |
| Loss and decay       | The default rendered level falls roughly 20 dB in 0.7 s and 30 dB in 1 s. No compensating envelope or EQ was added. This audit read that as an acceptable tom-scale sustain; it was too long, and the loss law was rewritten the next day (PLAN.md S1/S2). A long-ringing coupled fundamental survives even that and is the model's one confirmed defect — PLAN.md §P10/N3. |
| Product level        | Correct contact removes the spurious broadband peak, so the physical-voice output gain was recalibrated after the microphone filter. Mechanical state and energy are unchanged.                                                                                                                                                                                             |
| Persistence          | App-state v6 migrates only the exact old shipped HIT.R position to the new central default. Deliberate user edits and every earlier parameter index are preserved.                                                                                                                                                                                                          |

## Evidence and regression boundary

Sofia Dahl’s measured 12-inch-tom study reports central playing as normal,
5.5–8 ms stick contact across dynamics, a low-frequency sustain with only a
few modes remaining after the attack, and progressively stronger high
frequencies for louder hits:

- S. Dahl, [“Spectral changes in the tom-tom related to striking
  force”](https://www.speech.kth.se/qpsr/1997/1997_38_1_059-065.pdf),
  TMH-QPSR 38(1), 1997.

The regression suite now asserts the measured contact endpoints, shorter
contact for greater hardness, zero Trigger allocations, a separately excited
resonant head, batter-side observation semantics, and a strongest default
sustain peak in the 90–130 Hz fundamental band. The synthetic calibration
fixture is version 2 because its predecessor encoded the faulty contact law.

This is still a reduced physical model. It does not claim a measured shell,
room, microphone, or commercial-drum preset. Those remain behind the
measurement gate in
[`physical-real-instrument-departures.md`](physical-real-instrument-departures.md),
which a licensed reference now partly — but only partly — satisfies.

Two of this audit's conclusions were reached by listening and by comparison
against the model itself, which is all that was available in 2026-07. A
provenance-carrying reference now exists
([`reference/CREDITS.md`](../reference/CREDITS.md)), and the instrument used to
score against it has been characterised: most of its terms do not reproduce
between two coincident channels of the same hit, so an audit finding expressed as
a partial frequency or a per-mode decay carries far more uncertainty than the
figures here suggest. Read
[`physical-objective-validation.md`](physical-objective-validation.md) before
re-running anything in this document as a check.
