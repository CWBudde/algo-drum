#import "style.typ": code-path, paper

#let revision = sys.inputs.at("revision", default: "working tree")

#show: paper.with(
  title: "Matching a Physical Drum Model to a Recording",
  subtitle: "A perceptual distance in readable units, and what it establishes about a modal tom",
  author: "Christian-W. Budde",
  revision: revision,
  abstract-body: [
    Fitting a physical instrument model to a recording needs an objective that a
    person can read. This paper describes one: a recording and a render are
    reduced to the same set of perceptually named quantities --- partial
    frequencies and per-partial ring times, a time-varying fractional-octave
    envelope, an amplitude envelope, pitch glide, attack balance, and coverage in
    both directions --- and each term of the distance is reported in its own
    unit, so that a result can be read against a tolerance rather than only
    against another run. Adoption is decided per term. The reference reduction
    measures inter-channel delay and aligns before averaging, which on this
    recording is the difference between five detected partials and sixteen. The
    search is deterministic given its seed, checkpointed, and refuses to resume
    across a change in the model or the measurement.

    Applied to a double-headed tom, the apparatus takes the distance from 32.6 at
    the shipped default to 11.8, bringing five of the nine terms below their
    thresholds --- amplitude envelope, glide, attack balance and both coverage
    shares --- and it localises what remains. The model rings too long: its ring
    times sit above the reference across the whole band, its late spectrum runs
    20--30 dB hot above 1 kHz, and the head-damping parameter is pinned against
    its lower bound. That is a specific, testable statement about the model's
    damping range, and it is the kind of statement the apparatus exists to
    produce.
  ],
  status-body: [
    The fitted numbers here are an interim snapshot from a search still in
    progress, reported to show what the measurement yields rather than as a
    converged result. The method, the figures and the properties in
    #link(<reference>)[Section 5] and #link(<results>)[Section 7] do not depend
    on that search completing.
  ],
)

#set text(size: 8.8pt)
#set par(leading: 0.5em)

= The problem

Fitting a physically parameterised synthesiser to a recording is an optimisation
whose objective is the whole difficulty. A waveform difference is unusable
against a recording of a different physical instrument. A spectral distance is
usable but opaque: it produces one number, and when that number stops improving
there is nothing to say about what is still wrong.

The apparatus described here takes the opposite approach. Every term is a
quantity a listener would name, in the unit that quantity is perceived in, and
the sum exists only to order candidates --- decisions are made term by term. The
result is that a fit does not merely converge; it says what it has achieved and
what it has not, and the latter turns into statements about the model.

= The instrument and the target

The instrument is a double-headed tom rendered by modal synthesis: two membranes
coupled through a lumped air cavity, with a Berger tension nonlinearity, a
microphone model, and a three-band stochastic attack layer covering the transient
region modal synthesis reaches poorly. Its configuration is SI-valued and
versioned; the product exposes seventeen normalised parameters over it. Excitation
is selectable between a prescribed half-sine and a two-way coupled Hertzian
contact with Hunt--Crossley damping @hunt1975 @avanzini2001, whose contact time
follows the strike velocity rather than being imposed @dahl1997 @wagner2006.

The target is one recorded tom hit. Its provenance is a sample library, with no
documented microphone, room or processing chain, and no licence permitting
redistribution. It is therefore a *timbre-match target*, not an acoustic
validation reference: no automated test depends on it, and it is not part of the
repository. Keeping that distinction sharp matters, because some of what the
reduction reveals is a property of the recording rather than of any drum.

= Reduction: from a signal to named quantities

Reference and candidate pass through the same extraction, so any asymmetry
between them is a property of the signals rather than of the measurement.

*Partials* come from prominence-based peak picking on a 64k transform of the
sustain window. Prominence rather than a bare local-maximum test, because ripples
on the fundamental's skirt satisfy the latter and are not modes. Each surviving
peak carries a level, a ring time from a least-squares fit to its log envelope,
and that fit's $R^2$ --- which later lets a partial's frequency count while its
decay does not, since a beating pair or a mode buried under its neighbour has a
slope but not a meaningful one.

*Spectral envelope* is a 24-band third-octave vector computed over each of four
windows spanning the hit --- attack, early, body, tail --- so it describes not
only the timbre but the way the timbre evolves. *Amplitude envelope*, *pitch
glide* and *attack balance*, the click-to-body ratio, complete the reduction.

Deliberately absent is any sample-aligned waveform comparison. Against a
recording of a different physical drum, waveform error measures the phase
relationship between two signals that were never meant to share one: it is large
for candidates that sound identical and small for candidates that do not. Its
proper use, regression between two renders of the same model, is kept elsewhere.

= The distance

== Units and weights

Each term is reported in the unit its own perception is measured in: partial
frequency in cents, because the ear hears pitch as a ratio and 3 Hz means
something different at 118 Hz than at 1180; partial decay as
$abs(ln(T_60^"ref" \/ T_60^"cand"))$, because ringing twice as long and half as
long are the same size of mistake; levels and envelopes in dB; glide in cents.

Weights are the reciprocal of each term's "clearly wrong" threshold --- 25 cents
of pitch, 3 dB of partial balance, a factor of 1.4 in ring time, 4 dB of spectral
shape, 3 dB of envelope, 40 cents of glide, 6 dB of attack balance --- so a
just-audible error anywhere contributes about equally to the sum. Both signals
are peak-normalised, making every term gain-invariant; that is forced rather than
chosen, since the reference is normalised and its loudness carries no
information.

== Matching

Partials are matched greedily by closeness in cents rather than in index order,
so one badly placed candidate cannot cascade a misidentification through the
series, and each candidate is claimed at most once, so it cannot explain two
reference modes. The tolerance widens with mode index, since real two-headed
drums scatter about $plus.minus 20%$ around the ideal Bessel series in both
directions @richardson2012.

== Coverage, in both directions <coverage>

Six terms are computed over matched pairs, and an error averaged over matched
pairs is zero when there are no pairs. Each is therefore blended against a fixed
penalty in proportion to the share of the reference left unmatched, so that a
partial which is missing costs what a partial which is present but wrong costs.

That share must be weighted, and the weighting is the substantive choice.
Weighting by *energy* concentrates it on whichever partial is loudest --- on this
reference, one partial carries 99.4% of the total --- so a candidate holding that
partial alone reports almost nothing missing. Weighting by *count* overcorrects:
the reference contains a genuine, isolated component 39 dB down that no
two-headed drum will produce, and failing to reproduce it must not cost what
failing to reproduce the fundamental costs. Each partial is therefore worth *how
far it stands above the detection floor, in dB* --- zero at the floor, growing
with prominence, monotone in loudness like energy but compressed enough that
several missing quiet partials cannot be rounded away.

A second share scores the mirror case. Because matching iterates the reference, a
candidate partial with no reference counterpart is invisible to every partial
term and reaches the sum only through the spectral envelope, so a candidate can
cover the reference completely while its second-loudest component is a mode the
reference does not have. Both shares therefore appear, under the same weighting
and at the same weight. @partials-figure is the picture they score.

#figure(
  image("figures/partials.png", width: 100%),
  caption: [
    What the two coverage terms see. Grey links join matched pairs; shading marks
    the span between the lowest and highest reference partial, within which the
    spurious share is counted. The two reference partials without counterparts
    sit at $-41$ dB, close to the detection floor, and are correspondingly cheap
    to miss.
  ],
) <partials-figure>

The spurious share is counted *only* between the lowest and highest reference
partial. Outside that span the reference's own detection is unproven --- a
recording's noise floor hides modes a model legitimately has --- so a partial out
there is charged by the spectral envelope, on evidence, and not by coverage.
Without that bound the term would fit the recording's limitations rather than the
drum.

== The gate

The sum orders candidates; it does not decide them. Adoption is gated on three
terms individually --- partial frequency within 25 cents, partial decay within
0.25 in log ratio, spectral envelope within 4 dB --- because a lower sum can be
bought in terms nobody listens for.

That is not hypothetical. In one run the candidate with the *best* spectral
envelope of any measured had the *fewest* partials, three, having found that
raising the stochastic attack layer's level and dropping its corner frequency
into the band of interest satisfies a band-energy measure. Broadband noise can
satisfy a spectral envelope; it cannot produce resolvable partials. On the total
alone that run looked like the best match to the recording.

= Preparing the reference <reference>

A stereo recording of one hit is two views of the same event, and they are
generally not simultaneous. Summing them is then a comb filter: for a delay
$tau$,

$ H(f) = 2 abs(cos(pi f tau)) $ <comb-eq>

with nulls at $(2k+1)\/2tau$ and maxima at $k\/tau$ --- a fixed $plus.minus 6$ dB
spectral shape imposed by microphone geometry alone @blauert1997.

The reduction therefore measures the inter-channel lag by cross-correlation and
aligns before averaging. It fires only when the pair is genuinely one signal
twice --- correlation at least 0.5, and clearly better than summing --- and never
on short or uncorrelated input. The measured lag and correlation are reported on
the decoded reference, so the condition cannot pass unnoticed, and regression
tests pin both directions: a delayed pair is aligned, an uncorrelated pair is left
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
    @comb-eq. Shaded: 476--700 Hz, where the comb's first maximum falls.
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

Aligning also brings the reference's ring times into agreement with membrane
physics. Constant $Q$ requires $T_60 prop 1\/f$ @fletcher1998; the naive downmix
gives a median of 1.13 s in the 476--700 Hz band where that law predicts 0.31 s, a
factor of 3.6 that invites explanation in terms of a reverberant room. The aligned
reduction gives 0.16, 0.19, 0.60 and 0.61 s, which needs no such explanation.

= The search

The distance is minimised with the Mayfly Optimization Algorithm
@zervoudakis2020, a build-time-only dependency imported by the fitting command
and nothing else --- the shipped binary is unchanged. The search space is exactly
the bank the product exposes, plus strike velocity, with the quality tier pinned:
mode count buys fidelity with CPU and is a product decision rather than a property
of this drum.

Runs are deterministic given the seed. Progress is checkpointed with the completed
restarts and the running best point, written atomically, and the checkpoint
carries a fingerprint of the baseline cost. A resume across any change to the
model or the measurement is refused rather than silently mixed --- which has
repeatedly caught a stale checkpoint that would otherwise have produced a run
whose early generations were scored by one metric and its later ones by another.

One property of the model is worth stating here, because it determines the tier a
comparison must run at. Mode selection retains a fixed *count* --- 48, 96 or 160
by quality tier --- while membrane mode frequencies scale as $sqrt(T)$. Rendered
bandwidth is therefore a function of tier *and tuning* together, and it falls as
the drum is tuned down. For a bank at 331 N/m, roughly a quarter of the shipped
batter tension:

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

Two consequences follow: an instrument truncated by count loses its top octave
when it is tuned down, and any comparison between configurations must run at the
tier that ships. Bounding mode selection by frequency instead would make the
ceiling independent of tuning.

= What the fit establishes <results>

Against the right channel at the shipped Standard tier with Hertzian contact, the
distance falls from 32.585 at the shipped default to 11.784. More usefully,
@terms-figure shows where it fell.

#figure(
  image("figures/terms.png", width: 100%),
  caption: [
    Every term as a multiple of its own threshold; the three gated terms are
    highlighted. Five terms are brought below threshold, three remain between
    2.4$times$ and 2.9$times$, and the two coverage shares confirm the candidate
    is a drum rather than either degenerate extreme.
  ],
) <terms-figure>

Five of the nine terms are brought below threshold: amplitude envelope to
$0.27times$, glide $0.44times$, attack balance $0.15times$, and both coverage
shares --- unmatched $0.05times$, spurious $0.69times$. The coverage pair is the
useful confirmation: the candidate accounts for essentially all of the reference's
audible partial content while inventing little, so the remaining terms are being
computed over a genuine correspondence rather than over one lucky pair.

Three gated terms remain above threshold --- partial frequency $2.4times$, partial
decay $2.8times$, spectral envelope $2.9times$ --- and they are not three
independent problems. @decay-figure and @bands-figure show one.

#figure(
  image("figures/decay.png", width: 100%),
  caption: [
    Ring time against frequency, with the constant-$Q$ law anchored on the
    reference fundamental. The model's partials sit above the reference across the
    band, by a factor of 3--5 in the mid range.
  ],
) <decay-figure>

#figure(
  image("figures/bands.png", width: 100%),
  caption: [
    Spectral envelope, window by window. The attack matches closely; the
    discrepancy appears in the body and tail and is concentrated above 1 kHz,
    where the model runs 20--30 dB hot.
  ],
) <bands-figure>

*The model rings too long.* Its ring times exceed the reference's everywhere, by a
factor of 3--5 in the mid range, and the same fact appears in the spectral
envelope as a late-window excess above 1 kHz --- energy that should have decayed
is still present. The attack window matches within a few dB, so this is a decay
property and not a spectrum-shaping one.

*And the fit says why it cannot fix it.* Head damping is fitted to 0.276 on a
range whose lower bound is 0.25 --- normalised position 0.036, effectively pinned.
The optimiser spent its remaining freedom elsewhere, tuning the batter head down
to 935 N/m and tilting the damping law, because the parameter that would actually
shorten the ring has no travel left. This is a concrete and testable claim about
the model rather than about the recording: *the head-damping range does not reach
the losses this instrument exhibits.* Extending it, and checking whether the
partial-decay and spectral-envelope terms fall together, is the next experiment,
and the distance is arranged so that it will be a clear answer either way.

= Practical notes

Three points generalise beyond this instrument, each learned by measurement.

*Measure inter-channel lag before summing.* A stereo pair of one event is one
signal twice, and averaging imposes a fixed comb on every spectral quantity
downstream. A low zero-lag correlation between two views of one close-miked hit
is evidence that the channels are offset in time, not that there is a room on it.

*Score coverage both ways, and weight the two sides equally.* Either direction
alone admits a degenerate optimum. Weighting the invented-partial side more
heavily than the missing-partial side inverts the degeneracy rather than closing
it --- discarding the drum becomes the cheapest way to invent nothing, and a run
under such a weighting duly converged on two partials. The pressure toward
completeness and the pressure toward tidiness are one quantity seen from two
sides; the asymmetry belongs in the blend, where it is already present, and not
in the weights.

*Gate per term, and confirm by ear.* A distance can be monotone in quality ---
ranking candidates in the order a listener would --- while being wrong about all
of them in absolute terms. Ordering is much easier to get right than calibration,
and a converged search reports the former while appearing to report the latter.

= Scope and reproducibility

The model, the feature extraction, the distance and the search are in
#code-path("internal/physical"), #code-path("internal/physical/match") and
#code-path("cmd/fit-physical"). Runs are deterministic given the seed and are
excluded from continuous integration, since they take minutes and require a
recording the repository does not contain. The channel-alignment behaviour, the
audibility weighting and the relationship between the two coverage weights are
pinned by tests that need no recording.

@partials-figure, @terms-figure, @decay-figure and @bands-figure are drawn by
#code-path("cmd/paper-figures") --- `just paper-figures` --- from a single fit
report, which carries both feature sets in full, so they are reproducible from a
committed artefact rather than from the recording, which is not redistributable.
That command reproduces the distance's own greedy partial matching rather than
approximating it, so @partials-figure shows the pairing the score was actually
computed over. @comb-figure is measured from the two channels directly and is not
regenerable without them.

#show bibliography: set text(size: 8.1pt)
#bibliography("references.bib", style: "ieee", title: "References")
