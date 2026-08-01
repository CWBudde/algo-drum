# Reference recordings: licence and provenance

This directory is ignored wholesale by `.gitignore` except for the files listed
below, which are committed so that a fit can be reproduced from the repository
alone. Everything else here is a local working file and must not be committed.

## Committed set

**Source.** [Tomtom 08x08inch-multisampled](https://freesound.org/people/quartertone/packs/8767/),
uploaded to Freesound by **quartertone**.

**Licence.** Creative Commons Attribution 4.0 International (CC BY 4.0),
<https://creativecommons.org/licenses/by/4.0/>.

> You are free to share (to copy, distribute and transmit) and to remix (to adapt
> and modify) as long as you credit the author of the sound.

**Required attribution.** Any distribution of this repository, or of audio
derived from these files, must carry:

> Tom samples by *quartertone*, from the pack "Tomtom 08x08inch-multisampled"
> (<https://freesound.org/people/quartertone/packs/8767/>), used under CC BY 4.0.

**What was verified, and what was not.** The licence above was read directly from
the sound page for [ID 141976](https://freesound.org/people/quartertone/sounds/141976/)
(`TT08x08-VLP-Rm-v10.wav`), which is a member of this pack. Freesound assigns
licences per upload rather than per pack, and the per-sound pages for the
medium-pitch head strikes committed here were **not** individually opened. The
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

| Quantity | Value | Consequence for `cmd/fit-physical` |
| --- | --- | --- |
| Shell diameter | 8 in = 0.2032 m | `SIZE` becomes a constant |
| Shell depth | 8 in = 0.2032 m | `DEPTH` becomes a constant |
| Batter head | Remo coated Ambassador, 10 mil single ply | batter surface density is known, not fitted |
| Resonant head | Remo clear Diplomat, 7.5 mil single ply | resonant surface density likewise |

Head gauges are the manufacturer's standard for those models; the surface
densities that follow from them have not been measured here and the coating's
contribution is not accounted for.

## File naming

`tt08x08-<tuning>-<style>-v<velocity>.wav`

- tuning: `vlp` `lp` `mlp` `mp` `mhp` `hp` `vhp` — very low through very high pitch
- style: `hd` head strike, `rs` rimshot (head and rim together), `rm` rim strike
- velocity: `v01`–`v16`, soft to hard

Only **`mp` + `hd`** is committed — medium pitch, head strikes, all sixteen
velocities. That is the set the fit uses; the full pack is 336 files and 126 MB
against 5.5 MB for these.

## Measured properties of the committed files

Measured here, not claimed by the source:

- 48 kHz, 24-bit, stereo, 1.250 s per file. The engine runs at 48 kHz, so these
  need no resampling.
- The stereo pair is **coincident**. Peak inter-channel correlation falls at
  **exactly 0 samples** of lag, at 0.87–0.97 across the set. Summing the two
  channels to mono is therefore safe — there is no comb filter. This is not true
  of the older `tom.wav`, whose channels are 69 samples (1.56 ms) apart and
  correlate only 0.36 at zero lag, so summing it notches the spectrum.
- Whether the committed stereo pair is the Peavey XY pair or a mix including the
  Equitek close mic is **not stated by the source and not determinable from the
  files**. The zero-lag coincidence is consistent with the XY pair.
- `v05` and `v06` are near-duplicate takes: reduced through the analysis chain
  they agree to seven decimal places.
- Strike position, strike angle, microphone distance and microphone angle are
  **not** stated and remain fitted parameters.

## Checksums

SHA-256, first 16 hex digits:

| File | SHA-256 (truncated) |
| --- | --- |
| `tt08x08-mp-hd-v01.wav` | `261a87ed6543d9b5` |
| `tt08x08-mp-hd-v02.wav` | `4db659e2abceae6c` |
| `tt08x08-mp-hd-v03.wav` | `527443eaeef4d238` |
| `tt08x08-mp-hd-v04.wav` | `aff087b15a1ec604` |
| `tt08x08-mp-hd-v05.wav` | `15f878f23b7e3026` |
| `tt08x08-mp-hd-v06.wav` | `d7b1881b771f2b6b` |
| `tt08x08-mp-hd-v07.wav` | `cc48bff736e56108` |
| `tt08x08-mp-hd-v08.wav` | `4cbd404bd4cd935b` |
| `tt08x08-mp-hd-v09.wav` | `55cb42c5291c83c8` |
| `tt08x08-mp-hd-v10.wav` | `37bd114b4d8fbe56` |
| `tt08x08-mp-hd-v11.wav` | `a207f94a3d1f6119` |
| `tt08x08-mp-hd-v12.wav` | `967a29841daa1ba8` |
| `tt08x08-mp-hd-v13.wav` | `68afacc75573cda4` |
| `tt08x08-mp-hd-v14.wav` | `9f4caf3ba1d0b206` |
| `tt08x08-mp-hd-v15.wav` | `31be152e13573b15` |
| `tt08x08-mp-hd-v16.wav` | `2cd7dc50893e72b2` |

## `tom.wav` — the superseded reference

`tom.wav` is a 44.1 kHz stereo tom recording of **unknown provenance**: no
instrument, no dimensions, no tuning, no microphone, no room, no licence. It is
not committed and must not be, and it is the reason
`docs/physical-model-research.md` carries the rule against committing
third-party audio without clear licence and provenance.

Every fitted bank and every figure in `docs/paper/` currently derives from it.
It should be retired in favour of the set above, but deleting the file orphans
`testdata/physical-fit-tom.json`, the paper's figures and much of
`docs/physical-measured-fit.md` — all of which were derived from a recording
nobody can obtain. Keep it locally until those are re-derived against this set,
then retire it in the same change.
