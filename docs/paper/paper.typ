#import "style.typ": code-path, paper

#let revision = sys.inputs.at("revision", default: "working tree")

#show: paper.with(
  title: "The Target Was a Comb Filter",
  subtitle: "Measurement validity in fitting a modal drum model to a recorded tom",
  author: "Christian-W. Budde",
  revision: revision,
  abstract-body: [
    A modal two-headed drum model was fitted to one recorded tom by minimising an
    eight-term perceptual distance with a swarm optimiser, under each of two
    excitation models. The fits converged, ranked consistently, and agreed with a
    listening test on their ordering --- and none of them sounded like the
    recording. Auditing the measurement rather than the search found the reason.
    The reference is a stereo pair of one hit whose second channel lags the first
    by 69 samples at 44.1 kHz; the harness reduced it to mono by averaging, which
    is a comb filter with a notch at 320 Hz and a peak at 639 Hz. The dense cluster
    of nine partials between 476 and 700 Hz that had been treated as the model's
    outstanding deficiency is largely an artefact of that sum: de-combed, the
    reference has five detected partials with two in the band, against the model's
    five. Two further defects compounded it. The modal budget
    is a fixed mode *count*, so the rendered bandwidth scales with $sqrt(T)$ and
    the fitted bank --- tuned down to reach the reference's ring time --- has no
    modes at all above 467 Hz. And the excitation comparison ran at the tier where
    that ceiling binds, so the contact model whose advantage lies above 800 Hz was
    tested where it cannot act. The negative result stands as a caution: a distance
    that ranks candidates correctly can still be measuring the microphone geometry.
  ],
  status-body: [
    The fitted numbers in #link(<results>)[Section 3] were measured through the
    defect identified in #link(<comb>)[Section 4.1] and are reported as the
    evidence that prompted the audit, not as results. Section 5 lists what must be
    re-derived before any of them is believed.
  ],
)

#set text(size: 8.8pt)
#set par(leading: 0.5em)

= What is being fitted

The instrument is a real-time modal model of a two-headed tom: two circular
membranes coupled through a lumped air cavity, with a Berger tension
nonlinearity, a microphone model, and a three-band stochastic attack layer
standing in for the unresolvably dense high-frequency region. Seventeen
normalised parameters are exposed to the product --- diameter, both tensions,
damping and its frequency tilt, strike and pickup geometry, shell depth, cavity
coupling, nonlinearity, asymmetry, and the attack layer's level and corner
frequency --- and the fit searches exactly that bank, so that whatever it finds
is reachable by a user.

The target is a single recorded tom hit of *unknown provenance*: a sample-library
file with no record of the drum, head, tuning, mallet, microphone, room or gain
chain, and no licence permitting redistribution. It is therefore a timbre-match
target, not an acoustic validation reference, and no automated test depends on
it. That distinction turns out to matter more than it was expected to.

= Method

Reference and candidate are reduced to the same feature vector and scored by an
eight-term distance, each term in its own perceptual unit so that it can be read
against a tolerance rather than only against another run: partial frequency
(cents), partial level and spectral envelope (dB), partial decay (log ratio of
$T_60$), amplitude envelope (dB), pitch glide (cents), attack balance (dB), and
the share of reference partial energy left unmatched. Weights are the reciprocal
of each term's "clearly wrong" threshold, so a just-audible error anywhere
contributes about equally. Both signals are peak-normalised, so every term is
gain-invariant --- forced, since the reference is normalised and its loudness
carries no information.

Partials are found by prominence-based peak picking on a 64k transform of the
sustain window, capped at sixteen, and matched greedily by closeness in cents
with a tolerance that widens with mode index, since real two-headed drums scatter
about $plus.minus 20%$ around the ideal Bessel series in both directions
@richardson2012.

The search minimises this distance with the Mayfly Optimization Algorithm
@zervoudakis2020: eight independent restarts of 150 iterations at population 16,
59 056 objective evaluations per run, deterministic given the seed. Two
excitation models were compared. `ContactPrescribed` writes a half-sine of the
measured contact duration into a force buffer, so the head cannot influence it.
`ContactHertzian` integrates the stick as a free mass against a Hunt--Crossley
contact spring @hunt1975 @avanzini2001, calibrated against measured contact times
@dahl1997 @wagner2006.

= What the fits reported <results>

#figure(
  table(
    columns: 5,
    align: (left, right, right, right, left),
    table.header([Term], [Prescr.], [Hertz.], [Hertz. 5 g], [Gate]),
    [Partial frequency], [20.6 ¢], [19.6 ¢], [27.2 ¢], [$<= 25$ ¢],
    [Partial decay], [0.179], [0.493], [0.188], [$<= 0.25$],
    [Spectral envelope], [12.3 dB], [14.5 dB], [11.9 dB], [$<= 4$ dB],
    [Partial level], [2.0 dB], [2.0 dB], [2.0 dB], [---],
    [Amplitude envelope], [1.0 dB], [2.8 dB], [3.1 dB], [---],
    [Glide], [18.0 ¢], [0.1 ¢], [0.0 ¢], [---],
    [Attack balance], [0.0 dB], [0.0 dB], [1.1 dB], [---],
    [*Total*], [*5.901*], [*7.450*], [*6.548*], [---],
    [Partials in 476--700 Hz], [0], [0], [0], [9 in ref.],
  ),
  caption: [
    The three fits, each 8 restarts $times$ 150 iterations. The prescribed
    excitation wins on total and on two of three gate terms; none comes within
    three times the spectral-envelope gate. The 5 g row is the Hertzian model with
    the mallet reduced from the shipped 15 g to the mass its own velocity law
    implies.
  ],
) <results-table>

The ordering was confirmed by ear --- prescribed closest, then the 5 g Hertzian
--- which is the observation that makes the rest of this paper necessary: *the
distance ranks correctly and none of the candidates is close*. A metric can be
monotone in quality and still be measuring the wrong thing.

Two structural observations came out of the same runs. The Hertzian fit leaves
the attack layer's corner frequency `ATK.T` at 3426 Hz against the prescribed
fit's 1261 Hz, confirming a prediction that a physically excited high band
removes the search's need to fake one with noise. And the 5 g run achieves the
*best* spectral envelope of any fit while finding the *fewest* partials --- three
--- by parking `ATK.T` at 626 Hz, inside the disputed band, with the attack level
raised 3.7-fold. Broadband noise satisfies a band-energy metric and cannot make
resolvable partials. This is the argument for a per-term gate: on total alone
that run would have looked like the best match to the recording.

= Three defects, found by auditing the measurement

== The reference downmix is a comb filter <comb>

The reference is a two-channel file. Cross-correlating the channels shows they
are the same signal: correlation *0.942* at a lag of *69 samples* (1.56 ms at
44.1 kHz), against 0.361 at zero lag. The harness reduced multi-channel input to
mono by averaging, and averaging a signal with a delayed copy of itself is a comb
filter @blauert1997,

$ H(f) = 2 abs(cos(pi f tau)), $ <comb-eq>

with nulls at $(2k+1) \/ 2 tau$ and maxima at $k \/ tau$. For $tau = 1.565$ ms
those fall at 320, 959 and 1598 Hz, and at 639 and 1278 Hz respectively.
@comb-figure measures the downmix against a single channel and finds exactly that
envelope.

#figure(
  image("figures/comb.png", width: 100%),
  caption: [
    The mono downmix relative to the right channel alone, against
    @comb-eq with no fitted parameter. Shaded: the 476--700 Hz band.
  ],
) <comb-figure>

The consequence is not subtle. The comb's first null at 320 Hz hollows out the
200--476 Hz region, and its first maximum at 639 Hz sits in the middle of the
476--700 Hz band. With the partial detector capped at sixteen peaks, that shifts
which peaks survive:

#figure(
  table(
    columns: 3,
    align: (left, right, right),
    table.header([Reference analysed as], [Partials], [In 476--700 Hz]),
    [mono, averaged --- _every fit above_], [16], [*9*],
    [right channel alone], [7], [2],
    [mono, aligned then averaged], [*5*], [*2*],
  ),
  caption: [
    The same recording, three reductions. Aligning before summing brings the mono
    view into agreement with the single channels; the naive sum does not agree
    with either.
  ],
) <channel-table>

The dense cluster of nine partials between 476 and 700 Hz --- documented as the
recording's essential character, and treated as the model's outstanding
deficiency because no mode count or excitation could reproduce it --- is
substantially an artefact of the sum. De-combed, the reference has five detected
partials, two of them in that band. The model produces five.

The same defect distorted the decay measurements. @t60-figure shows the combed
reference's ring times against the $T_60 prop 1\/f$ that constant $Q$ requires of
a membrane @fletcher1998: in the disputed band the median is 1.13 s where
constant $Q$ predicts 0.31 s, a factor of 3.6. Read at the time as evidence of a
reverberant room, it is at least partly the comb's nulls beating against the
partials whose decay is being fitted --- the aligned reduction gives 0.16, 0.19,
0.60 and 0.61 s, which are membrane-like.

#figure(
  image("figures/t60.png", width: 100%),
  caption: [
    Ring time against frequency for the combed reference and the prescribed fit,
    with constant $Q$ anchored on the reference fundamental.
  ],
) <t60-figure>

The clue was present and read backwards. The provenance notes record "channel
correlation 0.36 --- there is a room on it". The correct reading of a
zero-lag correlation that low between two views of one close-miked hit is that
the channels are time-offset, and must not be summed.

== The modal bandwidth collapses when the drum is tuned down

Mode selection retains a fixed *count* --- 48, 96 or 160 by quality tier --- and
membrane mode frequencies scale as $sqrt(T)$. The fit lowers batter tension from
the shipped 1250 N/m to 331 N/m, because tension is the parameter that lengthens
the fundamental's ring at unchanged damping, and that halves the ceiling:

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header([Tier], [Modes], [Top mode], [In 476--700 Hz]),
    [Draft --- _used by the search_], [48], [*467 Hz*], [0],
    [Standard], [96], [664 Hz], [45],
    [High], [160], [852 Hz], [58],
  ),
  caption: [The fitted bank's modal ceiling by quality tier.],
) <ceiling-table>

The fitted drum has no modes at all above 467 Hz (@spectra-figure), and the
quality parameter is pinned during the search, so the optimiser cannot buy them
back. This is a modelling defect independent of the fit: a real membrane has
modes at every frequency, and truncating by count makes the rendered bandwidth a
function of the tuning, so the instrument loses its top octave when it is tuned
down.

#figure(
  image("figures/spectra.png", width: 100%),
  caption: [
    Sustain spectra. Red: detected partials. Blue ticks along the axis: the
    model's retained modes, which stop at 467 Hz. Above that the fitted bank has
    only the attack layer's noise floor.
  ],
) <spectra-figure>

== The excitation comparison was confounded

The Hertzian contact's measured advantage is $+11.8$ dB at 800 Hz, $+15.1$ at
1.5 kHz and $+22.9$ at 2.5 kHz, and only 0--4 dB below 700 Hz. Both fits ran at
the Draft tier, where the modal bank stops at 467 Hz --- so the improved
excitation had almost nothing modal to drive, and its whole advantage landed in a
region the model does not populate. Re-measuring the fitted banks at Standard,
the Hertzian configuration is the only one that has ever produced partials inside
the disputed band (three to four, reaching 1000 Hz), while the prescribed
configuration gains nothing from the extra modes, as its spectral zeros predict.

The conclusion that the Hertzian contact does not earn its calibration pass is
therefore not supported. It was tested where it cannot act.

= Consequences

Everything measured through the naive downmix must be re-derived: the fitted
banks, the committed fixture and preset, and the claim that the model cannot
populate 476--700 Hz. That claim was the highest-priority open item on this
model's roadmap, and the evidence for it is now largely attributable to the
measurement.

Three changes follow. First, the mono reduction measures the inter-channel lag
and aligns before averaging, guarded so that it fires only when the pair is
genuinely one signal twice --- correlation at least 0.5 and clearly better than
summing --- and never on short or uncorrelated input; the measured lag and
correlation are reported on the decoded reference so the condition cannot hide
again. Second, mode selection should be bounded by frequency rather than by
count, so that the ceiling does not move with the tuning. Third, the excitation
comparison must be repeated at a tier where the modal bank reaches the band the
contact model acts on.

The wider caution is the one this paper is named for. The fit was well-behaved
throughout: it converged, its restarts spread sensibly, its ranking was stable
across contact models and mallet masses, and a listening test agreed with its
ordering. None of that constrains whether the target is the instrument. A
distance is only as good as the reduction that produced it, and the reduction
here imposed a $plus.minus 6$ dB spectral shape --- a null and a peak, both fixed
by microphone spacing --- on every number in @results-table.

= Scope and reproducibility

The model, the feature extraction, the distance and the search are in
#code-path("internal/physical"), #code-path("internal/physical/match") and
#code-path("cmd/fit-physical"). Runs are deterministic given the seed and are
excluded from continuous integration, since they take minutes and require a
recording the repository does not contain. Figures here are generated from the
reference and from the committed fit reports; the recording itself is not
redistributable and is not part of the repository. The channel-alignment
behaviour is pinned by regression tests in both directions --- that a delayed
pair is aligned, and that an uncorrelated pair is left alone.

#show bibliography: set text(size: 8.1pt)
#bibliography("references.bib", style: "ieee", title: "References")
