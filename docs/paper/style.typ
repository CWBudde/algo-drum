#let ink = rgb("#17202a")
#let muted = rgb("#59636e")
#let accent = rgb("#006f7b")
#let rule = rgb("#c9d1d7")
#let draft-fill = rgb("#eef6f7")

#let abstract(body) = block(
  inset: (x: 10pt, y: 8pt),
  stroke: (left: 1.2pt + ink),
  fill: rgb("#f7f9fa"),
)[
  #strong[Abstract.] #body
]

// correction marks a chapter whose numbers were obtained under something since
// found to be faulty. It is deliberately as loud as draft-note: a reader who
// takes a number out of such a chapter must not be able to have missed it.
#let correction(body) = block(
  width: 100%,
  inset: 8pt,
  radius: 2pt,
  fill: draft-fill,
  stroke: 0.5pt + accent,
)[
  #body
]

#let draft-note(body) = block(
  width: 100%,
  inset: 8pt,
  radius: 2pt,
  fill: draft-fill,
  stroke: 0.5pt + accent,
)[
  #strong[Working-paper status.] #body
]

#let paper(
  body,
  title: "",
  subtitle: "",
  author: "",
  revision: "",
  abstract-body: none,
  status-body: none,
) = {
  set document(
    title: title,
    author: author,
    keywords: (
      "physical modelling",
      "modal synthesis",
      "drum acoustics",
      "parameter fitting",
      "measurement validity",
    ),
  )
  set page(
    paper: "a4",
    margin: (x: 18mm, top: 17mm, bottom: 19mm),
    numbering: "1",
    number-align: center,
  )
  set text(
    font: "Libertinus Serif",
    size: 9.2pt,
    fill: ink,
    lang: "en",
  )
  set par(justify: true, leading: 0.56em)
  set heading(numbering: "1.")
  set math.equation(numbering: "(1)")
  set table(
    stroke: rule,
    inset: (x: 5pt, y: 4pt),
  )
  show heading.where(level: 1): it => {
    v(0.55em)
    let label = if it.numbering == none {
      upper(it.body)
    } else {
      [
        #counter(heading).display(it.numbering) #upper(it.body)
      ]
    }
    block[
      #set par(justify: false)
      #text(
        font: "DejaVu Sans",
        size: 10pt,
        weight: "bold",
        fill: ink,
        label,
      )
    ]
  }
  show heading.where(level: 2): it => {
    v(0.35em)
    text(size: 10.2pt, weight: "bold", fill: ink, it)
  }
  show link: it => text(fill: ink, it)
  show raw.where(block: false): it => box(
    fill: rgb("#f1f3f5"),
    inset: (x: 2pt, y: 1pt),
    radius: 1.5pt,
    it,
  )

  align(center)[
    #text(size: 17pt, weight: "bold", title)
    #v(0.3em)
    #text(size: 10.5pt, fill: muted, subtitle)
    #v(0.8em)
    #text(weight: "bold", author)
    #linebreak()
    #text(size: 8pt, fill: muted)[Repository revision: #revision]
  ]

  v(0.65em)
  line(length: 100%, stroke: 0.8pt + ink)
  v(0.65em)

  if abstract-body != none {
    abstract(abstract-body)
    v(0.55em)
  }
  if status-body != none {
    draft-note(status-body)
    v(0.7em)
  }

  columns(2, body, gutter: 5mm)
}

// breakable-identifier-length is the length beyond which a code path is given
// explicit break opportunities. A column of this two-column layout fits about
// fifty monospace characters at the size below, so anything much longer than
// this overflows its container and overprints the text beside it.
#let breakable-identifier-length = 40

// code-path renders an identifier or repository path inline.
//
// Typst offers no line break inside a word, so the longest test names here would
// otherwise run off the column. Those are given zero-width spaces at the
// boundaries a reader would break at anyway: after a path separator or a dot,
// and between a lower-case character and the upper-case one that starts the next
// word of a camel-case name.
//
// The threshold matters for more than typography. A zero-width space survives
// into the text layer, so copying a treated identifier out of the PDF yields a
// string that will not match in a search. Applying the treatment only where the
// alternative is an unreadable page keeps every short path — which is most of
// them — exactly copyable.
#let code-path(path) = {
  let rendered = if path.len() <= breakable-identifier-length {
    path
  } else {
    path
      .replace(
        regex("([a-z0-9])([A-Z])"),
        match => match.captures.at(0) + "\u{200B}" + match.captures.at(1),
      )
      .replace(regex("([/.])"), match => match.captures.at(0) + "\u{200B}")
  }
  text(size: 7.8pt, raw(rendered))
}
