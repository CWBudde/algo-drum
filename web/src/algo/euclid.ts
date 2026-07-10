// Euclidean rhythm generator (Bjorklund's algorithm).
//
// E(pulses, steps) distributes `pulses` onsets as evenly as possible across
// `steps` slots — the family of rhythms behind countless drum patterns
// (E(3,8) = tresillo, E(5,8) = cinquillo, E(5,16), …). Pure and dependency-free
// so it can be unit-tested in isolation.

// euclid returns a boolean array of length `steps` with `pulses` onsets spread
// as evenly as possible, then rotated left by `rotation` slots. Inputs are
// clamped: pulses to [0, steps], steps to >= 0, rotation wraps modulo steps.
export function euclid(pulses: number, steps: number, rotation = 0): boolean[] {
  const n = Math.max(0, Math.floor(steps));
  if (n === 0) return [];

  const k = Math.max(0, Math.min(n, Math.floor(pulses)));
  if (k === 0) return new Array<boolean>(n).fill(false);
  if (k === n) return new Array<boolean>(n).fill(true);

  // Bjorklund: repeatedly fold the shorter "remainder" group into the longer
  // "counts" group until at most one remainder group is left.
  let counts: boolean[][] = [];
  for (let i = 0; i < k; i++) counts.push([true]);

  let remainders: boolean[][] = [];
  for (let i = 0; i < n - k; i++) remainders.push([false]);

  while (remainders.length > 1) {
    const pairs = Math.min(counts.length, remainders.length);
    const nextCounts: boolean[][] = [];
    for (let i = 0; i < pairs; i++) {
      nextCounts.push([...counts[i], ...remainders[i]]);
    }

    const nextRemainders =
      counts.length > remainders.length
        ? counts.slice(pairs)
        : remainders.slice(pairs);

    counts = nextCounts;
    remainders = nextRemainders;
  }

  const flat = [...counts, ...remainders].flat();

  // Rotate left so the first onset can be shifted around the cycle.
  const shift = ((Math.floor(rotation) % n) + n) % n;
  const out = new Array<boolean>(n);
  for (let i = 0; i < n; i++) {
    out[i] = flat[(i + shift) % n];
  }

  return out;
}
