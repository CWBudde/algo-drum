#import "style.typ": code-path, paper

#let revision = sys.inputs.at("revision", default: "working tree")

#show: paper.with(
  title: "Matching a Physical Drum Model to a Recording",
  subtitle: "A perceptual distance, a reproducible search, and what each of them establishes",
  author: "Christian-W. Budde",
  revision: revision,
  abstract-body: [
    This paper describes the measurement apparatus built to fit a modal
    two-headed drum model to a recorded tom, and what that apparatus has
    established. A recording and a render are reduced to the same set of
    perceptually named quantities --- partial frequencies, per-partial ring
    times, a time-varying fractional-octave envelope, an amplitude envelope,
    pitch glide and attack balance --- and compared by a distance whose every
    term carries its own unit, so that a result can be read against a tolerance
    rather than only against another run. Adoption is decided per term, not on
    the sum. The reference reduction measures inter-channel delay and aligns
    before averaging, which on this recording is the difference between five
    detected partials and sixteen: the two channels of the hit are one signal
    69 samples apart, and averaging them is a comb filter with a null at 320 Hz
    and a peak at 639 Hz. Partial coverage is scored in both directions and
    weighted by audibility above the detection floor. The search is
    deterministic given its seed and checkpointed, and the checkpoint refuses to
    resume across a change in the model or the measurement. Two properties of
    the model were established along the way: its rendered bandwidth is set by a
    mode *count* and therefore moves with the tuning, and the de-combed
    reference's ring times follow the $T_60 prop 1\/f$ that constant $Q$ requires
    of a membrane. The closing section collects the cautions --- each one found
    by measurement --- that a similar apparatus should be built to avoid.
  ],
  status-body: [
    This is a description of method and of established properties, not a report
    of a fitted bank. Fitting under the reduction and distance described here is
    in progress; the earlier fitted numbers were measured through the reference
    reduction corrected in #link(<reference>)[Section 4] and are not quoted as
    results.
  ],
)

#set text(size: 8.8pt)
#set par(leading: 0.5em)

= What is being matched

The instrument is a double-headed tom rendered by modal synthesis: two membranes
coupled through a lumped air cavity, with a Berger tension nonlinearity, a
microphone model, and a three-band stochastic attack layer covering the transient
region modal synthesis reaches poorly. Its configuration is SI-valued and
versioned, and the product exposes seventeen normalised parameters over it.

The target is one recorded tom hit. Its provenance is a sample library, with no
documented microphone, room or processing chain, and no licence permitting
redistribution. It is therefore a *timbre-match target*, not an acoustic
validation reference: no automated test depends on it, and it is not part of the
repository. Keeping that distinction sharp turned out to matter more than
expected, because several of the properties reported below are properties of the
recording rather than of any drum.

= Reducing a signal to what a listener would name

Reference and candidate pass through the same extraction, so any asymmetry
between them is a property of the signals and not of the measurement.

*Partials* are found by prominence-based peak picking on a 64k transform of the
sustain window. Prominence rather than a bare local-maximum test, because ripples
on the fundamental's skirt satisfy the latter and are not modes. Each surviving
peak gets a ring time from a least-squares fit to its log envelope, together with
the fit's $R^2$, which later lets a partial's frequency count while its decay does
not --- a beating pair or a mode buried under its neighbour has a slope, but not a
meaningful one.

*Spectral envelope* is a fractional-octave band vector computed per window across
the hit, so it describes not just the timbre but the way the timbre evolves.
*Amplitude envelope*, *pitch glide* and *attack balance* --- the click-to-body
ratio --- complete the reduction.

Deliberately absent is any sample-aligned waveform comparison. Against a room
recording of a different physical drum, waveform error measures the phase
relationship between two signals that were never meant to share one: it is large
for candidates that sound identical and small for candidates that do not. Its
proper use, regression between two renders of the same model, is kept elsewhere.

= A distance with readable units

Each term is reported in the unit its own perception is measured in: partial
frequency in cents, because the ear hears pitch as a ratio; partial decay as
$abs(ln(T_60^"ref" \/ T_60^"cand"))$, because ringing twice as long and half as
long are the same size of mistake; levels and envelopes in dB; glide in cents.
Two further terms score *partial coverage*, discussed below. Weights are the
reciprocal of each term's "clearly wrong" threshold --- 25 cents of pitch, 3 dB of
partial balance, a factor of 1.4 in ring time, 4 dB of spectral shape --- so a
just-audible error anywhere contributes about equally to the sum.

Both signals are peak-normalised, which makes every term gain-invariant. That is
forced rather than chosen: the reference is normalised and its loudness carries
no information.

Partials are matched greedily by closeness in cents rather than in index order,
so one badly placed candidate cannot cascade a misidentification through the
series, and each candidate is claimed at most once, so it cannot explain two
reference modes. The tolerance widens with mode index, since real two-headed
drums scatter about $plus.minus 20%$ around the ideal Bessel series in both
directions @richardson2012.

== Adoption is decided per term

The sum orders candidates; it does not decide them. Adoption is gated on three
terms individually --- partial frequency within 25 cents, partial decay within
0.25 in log ratio, spectral envelope within 4 dB --- because a lower sum can be
bought in terms nobody listens for.

This is not hypothetical. In one run the candidate with the *best* spectral
envelope of any measured had the *fewest* partials, three, having discovered that
raising the stochastic attack layer's level and dropping its corner frequency
into the band of interest satisfies a band-energy measure. Broadband noise can
satisfy a spectral envelope; it cannot produce resolvable partials. On the total
alone that run looked like the best match to the recording.

== Coverage is scored in both directions, by audibility <coverage>

Six of the terms are computed over matched partial pairs, and an error averaged
over matched pairs is zero when there are no pairs. Each is therefore blended
against a fixed penalty in proportion to the share of the reference left
unmatched, so that a partial which is missing costs what a partial which is
present but wrong costs.

That blend is only as good as the share driving it, and the share must be
weighted. Weighting by *energy* concentrates it on whichever partial is loudest
--- on this reference, one partial carries 99.4% of it --- and a candidate holding
that partial alone then reports almost nothing missing. Weighting by *count*
overcorrects in the other direction: this reference contains a genuine, isolated
component 39 dB down that no two-headed drum will produce, and failing to
reproduce it must not cost what failing to reproduce the fundamental costs. Each
partial is therefore worth *how far it stands above the detection floor, in dB*:
zero at the floor, growing with prominence, monotone in loudness like energy but
compressed enough that several missing quiet partials cannot be rounded away.

The mirror term matters as much. Because matching iterates the reference, a
candidate partial with no reference counterpart is invisible to every partial
term and reaches the sum only through the spectral envelope. A candidate can then
cover the reference completely while its second-loudest component is a mode the
reference does not have. The distance therefore carries both shares under the
same weighting.

The second is counted only between the lowest and highest reference partial.
Outside that span the reference's own detection is unproven --- a recording's
noise floor hides modes a model legitimately has --- so a partial out there is
charged by the spectral envelope, on evidence, and not by coverage. Without that
bound the term fits the recording's limitations rather than the drum.

The two shares carry equal weight, and the asymmetry the argument seems to call
for --- that nothing else absorbs an invented partial, while a missing one is
charged twice --- is already supplied by the blend. Supplying it a second time
through the weight makes an empty bank the cheapest way to invent nothing, which
a run demonstrated directly. #link(<cautions>)[Section 7] returns to this.

= Preparing the reference <reference>

A stereo recording of one hit is two views of the same event, and they are
generally not simultaneous. Summing them is then a comb filter: for a delay
$tau$, the sum has magnitude

$ H(f) = 2 abs(cos(pi f tau)) $ <comb-eq>

with nulls at $(2k+1)\/2tau$ and maxima at $k\/tau$ --- a fixed $plus.minus 6$ dB
spectral shape imposed by microphone geometry alone @blauert1997.

The reduction therefore measures the inter-channel lag by cross-correlation and
aligns before averaging. It fires only when the pair is genuinely one signal
twice --- correlation at least 0.5, and clearly better than summing --- and never
on short or uncorrelated input; the measured lag and correlation are reported on
the decoded reference, so the condition cannot go unnoticed. Regression tests pin
both directions: a delayed pair is aligned, and an uncorrelated pair is left
alone.

On this recording the second channel lags the first by 69 samples at 44.1 kHz,
1.56 ms, with correlation 0.942 at that lag against 0.361 at zero. @comb-eq then
predicts nulls at 320, 959 and 1598 Hz and maxima at 639 and 1278 Hz, which is
what the naive downmix shows against a single channel, with no fitted parameter
(@comb-figure).

#figure(
  image("figures/comb.png", width: 100%),
  caption: [
    The naive mono downmix relative to the right channel alone, against
    @comb-eq. Shaded: the 476--700 Hz band, which the comb's first maximum
    falls inside.
  ],
) <comb-figure>

The effect on the reduction is large, because the partial detector keeps a
bounded number of peaks and the comb decides which survive:

#figure(
  table(
    columns: 3,
    align: (left, right, right),
    table.header([Reference reduced as], [Partials], [In 476--700 Hz]),
    [mono, averaged naively], [16], [9],
    [right channel alone], [7], [2],
    [mono, aligned then averaged], [*5*], [*2*],
  ),
  caption: [
    One recording, three reductions. Aligning before summing brings the mono view
    into agreement with the single channels; the naive sum agrees with neither.
  ],
) <channel-table>

The aligned reduction also brings the reference's ring times into agreement with
membrane physics. Constant $Q$ requires $T_60 prop 1\/f$ of a membrane
@fletcher1998; the naive downmix gives a median of 1.13 s in the 476--700 Hz band
where that law predicts 0.31 s, a factor of 3.6 that invites explanation in terms
of a reverberant room. The aligned reduction gives 0.16, 0.19, 0.60 and 0.61 s,
which need no such explanation (@t60-figure).

#figure(
  image("figures/t60.png", width: 100%),
  caption: [
    Ring time against frequency, with constant $Q$ anchored on the reference
    fundamental. The upper trace is the naively summed reference; a fitted bank
    from an earlier run is shown for comparison of method.
  ],
) <t60-figure>

= Bandwidth is a consequence of tuning

Mode selection retains a fixed *count* --- 48, 96 or 160 by quality tier --- while
membrane mode frequencies scale as $sqrt(T)$. The rendered bandwidth is therefore
not a property of the tier alone but of the tier and the tuning together, and it
falls as the drum is tuned down. For a bank tuned to 331 N/m, roughly a quarter of
the shipped batter tension:

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header([Tier], [Modes], [Top mode], [In 476--700 Hz]),
    [Draft], [48], [467 Hz], [0],
    [Standard --- _shipped default_], [96], [664 Hz], [45],
    [High], [160], [852 Hz], [58],
  ),
  caption: [Modal ceiling of a low-tuned bank, by quality tier.],
) <ceiling-table>

This is worth stating as a property of the model rather than of any fit
(@spectra-figure). A real membrane has modes at every frequency; truncating by
count means an instrument loses its top octave when it is tuned down, and it means
any comparison between configurations must be run at the tier that ships. Bounding
mode selection by frequency instead would make the ceiling independent of tuning.

#figure(
  image("figures/spectra.png", width: 100%),
  caption: [
    Sustain spectra. Red: detected partials. Blue ticks: the model's retained
    modes for a low-tuned bank at the Draft tier, which stop at 467 Hz.
  ],
) <spectra-figure>

= The search

The distance is minimised with the Mayfly Optimization Algorithm
@zervoudakis2020, a build-time-only dependency imported by the fitting command
and nothing else --- the shipped binary is unchanged. The search space is exactly
the bank the product exposes, plus strike velocity, with the quality tier pinned,
since mode count buys fidelity with CPU and is a product decision rather than a
property of this drum.

Runs are deterministic given the seed. Progress is checkpointed with the
completed restarts and the running best point, written atomically, and the
checkpoint carries a fingerprint of the baseline cost. A resume across any change
to the model or the measurement is refused rather than silently mixed --- which
has twice caught a stale checkpoint that would otherwise have produced a run whose
early generations were scored by one metric and its later ones by another.

Excitation is selectable: a prescribed half-sine, and a two-way coupled Hertzian
contact with Hunt--Crossley damping @hunt1975 @avanzini2001, whose contact time
follows the strike velocity rather than being imposed @dahl1997 @wagner2006.

= What to watch for <cautions>

Each of the following was found by measurement, on this apparatus, and each cost
a run to find. They are collected here because none of them announces itself in a
converged, well-behaved, reproducible fit.

*Do not sum the channels of a single hit before measuring the lag.* A stereo pair
of one event is one signal twice, and averaging imposes a fixed comb on every
spectral quantity downstream. The clue here was present and read backwards: the
provenance notes recorded a channel correlation of 0.36 as evidence of a room,
where the correct reading of a low zero-lag correlation between two views of one
close-miked hit is that the channels are offset in time.

*Check where the model's bandwidth actually ends, at the tier under test.* A
comparison between excitation models is uninformative if it runs where the modal
bank stops below the band the excitation acts on; two models that differ chiefly
above 800 Hz cannot be told apart by a bank ending at 467 Hz.

*Weight coverage by audibility, and score it in both directions.* Energy
weighting collapses onto the loudest partial on any reference with a dominant
mode; scoring only what the reference has leaves invented modes free. Either
alone admits a degenerate optimum, in opposite directions.

*And do not overweight the second of those.* Setting the invented-partial weight
above the missing-partial weight makes discarding the drum the cheapest way to
invent nothing: a run under such a weighting converged on two partials. The
pressure toward completeness and the pressure toward tidiness are the same
quantity seen from two sides, and the balance between them belongs in the blend
rather than in the weights.

*Gate per term, and confirm by ear.* A distance can be monotone in quality ---
ranking candidates in the order a listener would --- and still be wrong about all
of them in absolute terms. Ordering is much easier to get right than calibration,
and a converged search reports the former while appearing to report the latter.

= Scope and reproducibility

The model, the feature extraction, the distance and the search are in
#code-path("internal/physical"), #code-path("internal/physical/match") and
#code-path("cmd/fit-physical"). Runs are deterministic given the seed and are
excluded from continuous integration, since they take minutes and require a
recording the repository does not contain. The channel-alignment behaviour, the
audibility weighting, and the relationship between the two coverage weights are
pinned by tests that need no recording. Figures are generated from the reference
and from committed fit reports; the recording itself is not redistributable.

#show bibliography: set text(size: 8.1pt)
#bibliography("references.bib", style: "ieee", title: "References")
