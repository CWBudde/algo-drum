# Reference recordings: licence and provenance

This directory is ignored wholesale by `.gitignore` except for the files listed
below, which are committed so that a fit can be reproduced from the repository
alone. Everything else here is a local working file and must not be committed.

## The three packs

Three multisampled tom packs are present locally, all by the same recordist and
all CC BY 4.0. Only sixteen files from the first are committed; the other two are
local working material, present so that a fit can be checked against a second and
third drum of known geometry.

| Directory  | Pack                                                                                  | Files | Committed            |
| ---------- | ------------------------------------------------------------------------------------- | ----: | -------------------- |
| `tt08x08/` | [Tomtom 08x08inch-multisampled](https://freesound.org/people/quartertone/packs/8767/) |   335 | `lp/hd/v01..v16.wav` |
| `tt10x08/` | [Tomtom10x08inch-multisampled](https://freesound.org/people/quartertone/packs/8117/)  |   144 | none                 |
| `tt12x08/` | [Tomtom12x08inch-multisampled](https://freesound.org/people/quartertone/packs/8127/)  |   188 | none                 |

The 10" and 12" packs ship Freesound's own `_readme_and_license.txt`, which lists
a URL and a licence **per sound**. Those two files are committed, together with a
generated `SOURCES.tsv` per pack mapping each reorganised path back to the
Freesound ID it came from — without it the rename below would destroy the only
link between a file on disk and the upload whose licence governs it. The 8" pack
was downloaded without such a manifest and has none.

## Committed set

**Source.** [Tomtom 08x08inch-multisampled](https://freesound.org/people/quartertone/packs/8767/),
uploaded to Freesound by **quartertone**.

**Licence.** Creative Commons Attribution 4.0 International (CC BY 4.0),
<https://creativecommons.org/licenses/by/4.0/>.

> You are free to share (to copy, distribute and transmit) and to remix (to adapt
> and modify) as long as you credit the author of the sound.

**Required attribution.** Any distribution of this repository, or of audio
derived from these files, must carry:

> Tom samples by _quartertone_, from the pack "Tomtom 08x08inch-multisampled"
> (<https://freesound.org/people/quartertone/packs/8767/>), used under CC BY 4.0.

**What was verified, and what was not.** The licence above was read directly from
the sound page for [ID 141976](https://freesound.org/people/quartertone/sounds/141976/)
(`TT08x08-VLP-Rm-v10.wav`), which is a member of this pack. Freesound assigns
licences per upload rather than per pack, and the per-sound pages for the
low-pitch head strikes committed here were **not** individually opened. The
whole pack was uploaded together by one author and is stated as CC BY 4.0, but if
this matters for a redistribution decision, check the individual sound pages
first rather than relying on this note.

## The instrument, as described by the recordist

> A 1-shot sample taken of an 8-inch diameter, 8-inch deep tomtom, recorded with
> an Equitek E100 close-mic and two Peavey electret condenser microphones in an
> XY pair. The drum was fitted with a Remo coated ambassador head on top and a
> Remo clear diplomat head on bottom.

This is the first reference in the project with a stated instrument. It fixes
values that were previously fitted parameters:

| Quantity       | Value                                     | Consequence for `cmd/fit-physical`          |
| -------------- | ----------------------------------------- | ------------------------------------------- |
| Shell diameter | 8 in = 0.2032 m                           | `SIZE` becomes a constant                   |
| Shell depth    | 8 in = 0.2032 m                           | `DEPTH` becomes a constant                  |
| Batter head    | Remo coated Ambassador, 10 mil single ply | batter surface density is known, not fitted |
| Resonant head  | Remo clear Diplomat, 7.5 mil single ply   | resonant surface density likewise           |

Head gauges are the manufacturer's standard for those models; the surface
densities that follow from them have not been measured here and the coating's
contribution is not accounted for.

## Layout

    reference/<kit>/<tuning>/<style>/v<velocity>.wav

- kit: `tt08x08` `tt10x08` `tt12x08` — shell diameter × depth, in inches
- tuning: `vlp` `lp` `mlp` `mp` `mhp` `hp` `vhp` — very low through very high pitch
- style: `hd` head strike, `rs` rimshot (head and rim together), `rm` rim strike;
  the 12" pack additionally distinguishes `hdc` head-centre from `hde` head-edge
  and has no plain `hd` at those tunings
- velocity: `v01`–`v16`, soft to hard

Every pack ships one flat directory. The 8" one names files
`tt08x08-<tuning>-<style>-v<NN>.wav`; the other two carry Freesound's upload
prefix as well, `<id>__quartertone__tt10x8-<tuning>-<style>-v<NN>.wav`. This tree
splits that name into directories and keeps only the velocity in the filename, so
`reference/tt08x08/lp/hd/v08.wav` is the eighth-hardest low-pitch head strike on
the 8" drum. A file carries no identity outside the tree, which is the cost of the
shorter paths — this document and the `SOURCES.tsv` files are where that identity
lives.

Not every tuning was recorded at every strike style, so the grids are ragged: the
8" pack has all seven tunings, the 10" only four, the 12" four with a different
style set. Three velocity slots are missing across the three packs, all of them
the source's doing rather than this repository's — see "Gaps" below.

Only **`tt08x08/lp/hd`** is committed — the 8" drum, low pitch, head strikes, all
sixteen velocities. That is the set the fit uses; the three packs together are
~320 MB against 9.6 MB for these sixteen.

Low pitch, not the medium set this project started on: it was chosen on the
sound. It happens also to be the more measurable of the two — see the note below
the checksums.

## Gaps

Three velocity slots are missing from the local tree. None of them is in the
committed set.

- **`tt08x08/vlp/rs/v10`** — never present. The 8" pack advertises 336 files and
  the download contains 335.
- **`tt12x08/mp/rs/v09`–`v12`** — never present under those labels. That group
  ships sixteen files, but the source labels four of them **twice**: two distinct
  uploads each for `v05`, `v06`, `v07` and `v08`, and nothing for `v09`–`v12`.
  The duplication is in the pack, not in the reorganisation.
- **`tt12x08/mp/rs`, four files lost locally (2026-08-01).** Because of the
  duplicate labels above, the flattening rename mapped two source files onto one
  destination path four times and the second overwrote the first. Freesound IDs
  **129973, 129974, 129975 and 129976** were destroyed; 129977–129980 survive and
  are what now sit at `v05`–`v08`. Nothing committed, generated or tested depended
  on them. They are CC BY 4.0 and individually downloadable from the URLs in
  `tt12x08/SOURCES.tsv`, where all eight are listed and the four lost ones are
  marked `LOST-collision`. Re-fetching them needs a naming decision — the labels
  they came with collide by construction.

## Measured properties of the committed files

Measured here, not claimed by the source:

- 48 kHz, 24-bit, stereo, 2.083 s per file. The engine runs at 48 kHz, so these
  need no resampling.
- The stereo pair is **coincident**. Peak inter-channel correlation falls at
  0 samples of lag on thirteen of the sixteen and 1 sample on the other three, at
  **0.944–0.990** across the set. One sample at 48 kHz is 21 µs, whose first null
  sits at 24 kHz, so summing the two channels to mono is safe — there is no comb
  filter in band. This was not true of the older `tom.wav`, whose channels were
  69 samples (1.56 ms) apart and correlated only 0.36 at zero lag, so summing it
  notched the spectrum.
- Whether the committed stereo pair is the Peavey XY pair or a mix including the
  Equitek close mic is **not stated by the source and not determinable from the
  files**. The near-zero-lag coincidence is consistent with the XY pair.
- No two takes in this set are duplicates: the closest adjacent pair correlates
  well below 0.99. (The medium-pitch set previously committed here did contain
  one such pair, `v05`/`v06`, which agreed to seven decimal places through the
  analysis chain. That was a property of that set, not of the pack.)
- Strike position, strike angle, microphone distance and microphone angle are
  **not** stated and remain fitted parameters.

## Checksums

SHA-256, first 16 hex digits:

All paths are relative to `reference/tt08x08/lp/hd/`.

| File      | SHA-256 (truncated) |
| --------- | ------------------- |
| `v01.wav` | `fc8d559cf5fd4eaa`  |
| `v02.wav` | `1a3fbe78d262feb4`  |
| `v03.wav` | `1e2bd2454c92414c`  |
| `v04.wav` | `b8df31d6d1a4b84d`  |
| `v05.wav` | `d62074f1a90915be`  |
| `v06.wav` | `671077d396357b36`  |
| `v07.wav` | `569871f23401a2b9`  |
| `v08.wav` | `e52be4376bfecc25`  |
| `v09.wav` | `1becd24bd6977642`  |
| `v10.wav` | `1184ad691b750537`  |
| `v11.wav` | `9195658dfc26f034`  |
| `v12.wav` | `bffb4e842cc5d4ca`  |
| `v13.wav` | `8f7ffc1b155a19da`  |
| `v14.wav` | `01f2cb8ff9079d4a`  |
| `v15.wav` | `0125b70b331a52e0`  |
| `v16.wav` | `8a63ac551368d7a6`  |

## Why the low-pitch set is also the better target

Chosen on the sound, but it is measurably the easier drum to fit, and the margin
is not small. `cmd/measure-objective` scores each channel of each take against
the other through the shipped `match.Distance`; the p90 of that disagreement is
the objective's own noise floor, and no fit can be asked to beat it:

| Term              | `mp/hd` p90 | `lp/hd` p90 | Gate now |
| ----------------- | ----------- | ----------- | -------- |
| Partial frequency | 76.2 ¢      | 65.5 ¢      | 70 ¢     |
| Partial level     | 6.81 dB     | 6.42 dB     | 7 dB     |
| Partial decay     | 0.558       | 0.535       | 0.55     |
| Spectral envelope | 3.67 dB     | 3.24 dB     | 3.5 dB   |
| Envelope          | 3.84 dB     | 1.38 dB     | 1.5 dB   |
| Glide             | 280.1 ¢     | **23.4 ¢**  | 30 ¢     |
| Attack balance    | 1.13 dB     | 0.81 dB     | 0.9 dB   |
| Unmatched share   | 0.250       | 0.223       | 0.25     |
| Spurious share    | 0.245       | 0.239       | 0.25     |

Same estimator, same code, different drum — except in the `lp/hd` column, which
was re-measured on 2026-08-01 after `PLAN.md` N17 re-sized the analysis and decay
windows, so the two columns are one estimator revision apart. Glide improves by a
factor of 12 because the glide estimator needs the fundamental to survive to its
late probe: on the medium-pitch takes it does not, so more than half of those
measurements were noise between two dead probes. (It read 2.3 ¢ — a factor of 120
— through the pre-N17 estimator, and that number is withdrawn.) The envelope term
improves because the files are 2.083 s rather than 1.250 s, so the tail being
compared is signal rather than floor.

One property of the low-pitch set that is _not_ an advantage, recorded here
because the pack's velocity series is the reason it was licensed: its glide
carries **no velocity trend** — all sixteen takes positive, median +18 ¢, three
unmeasurable — where the medium-pitch set's rises monotonically from −130 ¢ to
−353 ¢. A fit against `lp/hd` does not constrain the tension nonlinearity. See
[`docs/physical-objective-validation.md`](../docs/physical-objective-validation.md)
§Result 11e.

The gates in `internal/physical/match/DefaultWeights` are the right-hand column,
rounded up. They are a property of **this pair** — estimator _and_ recording —
and do not transfer to another tuning, another strike style or another pack.

## `tom.wav` — the superseded reference, deleted 2026-08-01

`tom.wav` was a 44.1 kHz stereo tom recording of **unknown provenance**: no
instrument, no dimensions, no tuning, no microphone, no room, no licence. It was
never committed and never could have been, and it is the reason
`docs/physical-model-research.md` carries the rule against committing
third-party audio without clear licence and provenance.

It was **deleted from the working tree on 2026-08-01** (`PLAN.md` P10/N8), ahead
of the re-derivation that was meant to accompany it. Nothing in the build or the
test suite ever opened it, so nothing broke; what the deletion did do is orphan
everything measured against it, none of which can now be re-measured:

- `testdata/physical-fit-tom.json` and the **Measured tom** preset in
  `web/src/algo/physicalTomPresets.ts` that mirrors it — a bank fitted to a
  recording that no longer exists, and one that missed all three adoption gates
  even while it did;
- the figures in `docs/paper/` and the totals the paper quotes;
- ~~`contactReferenceHz` = 118 Hz in the contact code~~ — **re-pointed
  2026-08-02** (`PLAN.md` N8a and N18). `contactReferenceHz` is no longer a
  constant: it is derived from the configuration under test, the retained (0,1)
  of `generateHeadModes`, which is 150.10 Hz on `DefaultPhysicalDrum()`. The
  literal 118s that survived N8a in `contact_test.go`'s analytic-pulse tests went
  with it, and the tables in
  [`docs/physical-contact.md`](../docs/physical-contact.md) were re-measured from
  the tests rather than adjusted;
- the historical dimensions quoted for it throughout `docs/physical-*.md` —
  44.1 kHz, channels 1.56 ms apart correlating 0.36 at zero lag — which are
  recorded measurements from while it existed and are no longer checkable.

**Not on that list, and deliberately: the 118.05 Hz partials in
`internal/physical/match/`.** `bench_test.go`, `esprit_test.go`, `glide_test.go`
and the worked example in `features.go`'s comments all put a 118 Hz fundamental
under a louder, shorter 212 Hz partial, which is this recording's layout and was
plainly taken from it — the last two say so in as many words. They are still not
anchors. Nothing there opens a file: each is a sum of exponentially decaying
sinusoids the test synthesizes and then measures itself, so every one of them
is checked against its own input rather than against the recording, and shifting
the whole table in pitch would change no result. The prose that names the
recording is a dated historical note of the kind this section asks for. Leave
them alone.

Re-deriving those against the licensed set above is `PLAN.md` N5 (the joint
sixteen-velocity fit) and N7 (the paper). Until then, treat every number
attributed to `tom.wav` anywhere in this repository as a dated historical note,
not as a claim anyone can verify.
