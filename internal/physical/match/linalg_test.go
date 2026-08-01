package match

import (
	"math"
	"math/cmplx"
	"testing"
)

// The linear algebra below has no second implementation in this repository to
// compare against, so every test here checks a defining property rather than a
// stored answer: an eigenpair satisfies A v = lambda v, an eigenbasis is
// orthonormal, a spectrum matches one that can be read off the matrix by
// inspection, a solved system reproduces its right-hand side. A stored table
// would only pin whatever these routines did the first time they ran.

func TestHermitianEigenSatisfiesItsDefinition(t *testing.T) {
	t.Parallel()

	matrix := [][]complex128{
		{complex(4, 0), complex(1, 2), complex(0, -1)},
		{complex(1, -2), complex(3, 0), complex(2, 1)},
		{complex(0, 1), complex(2, -1), complex(5, 0)},
	}

	values, vectors := hermitianEigen(matrix)

	if len(values) != 3 || len(vectors) != 3 {
		t.Fatalf("got %d values and %d vectors, want 3 of each", len(values), len(vectors))
	}

	for index := 1; index < len(values); index++ {
		if values[index] < values[index-1] {
			t.Errorf("eigenvalues are not ascending: %v", values)
		}
	}

	// A Hermitian matrix has real eigenvalues and its trace is their sum; both
	// are checks the decomposition cannot pass by accident.
	trace := real(matrix[0][0]) + real(matrix[1][1]) + real(matrix[2][2])

	sum := 0.0
	for _, value := range values {
		sum += value
	}

	if math.Abs(sum-trace) > 1e-9 {
		t.Errorf("eigenvalues sum to %v, want the trace %v", sum, trace)
	}

	for index, vector := range vectors {
		for row := range matrix {
			var applied complex128
			for column := range matrix {
				applied += matrix[row][column] * vector[column]
			}

			want := complex(values[index], 0) * vector[row]
			if cmplx.Abs(applied-want) > 1e-9 {
				t.Errorf("eigenpair %d row %d: A v = %v, lambda v = %v", index, row, applied, want)
			}
		}
	}

	for first := range vectors {
		for second := range vectors {
			var product complex128
			for row := range vectors[first] {
				product += cmplx.Conj(vectors[first][row]) * vectors[second][row]
			}

			want := complex128(0)
			if first == second {
				want = 1
			}

			if cmplx.Abs(product-want) > 1e-9 {
				t.Errorf("vectors %d and %d: inner product %v, want %v", first, second, product, want)
			}
		}
	}
}

func TestHermitianEigenHandlesADegenerateSpectrum(t *testing.T) {
	t.Parallel()

	// A repeated eigenvalue is where a Jacobi sweep can stall: the rotation
	// that should annihilate an off-diagonal entry is undetermined when the two
	// diagonal entries are equal. The identity's whole spectrum is degenerate.
	identity := [][]complex128{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}

	values, _ := hermitianEigen(identity)

	for index, value := range values {
		if math.Abs(value-1) > 1e-12 {
			t.Errorf("eigenvalue %d is %v, want 1", index, value)
		}
	}
}

func TestEigenvaluesMatchATriangularSpectrum(t *testing.T) {
	t.Parallel()

	// An upper triangular matrix wears its spectrum on its diagonal, so this
	// checks the Hessenberg reduction and the QR sweeps against an answer that
	// needs no computation.
	matrix := [][]complex128{
		{complex(2, 1), complex(7, -3), complex(-1, 4), complex(5, 5)},
		{0, complex(-3, 2), complex(6, 1), complex(2, -2)},
		{0, 0, complex(0.5, -4), complex(3, 0)},
		{0, 0, 0, complex(9, 0.25)},
	}

	want := []complex128{
		complex(2, 1), complex(-3, 2), complex(0.5, -4), complex(9, 0.25),
	}

	assertSpectrum(t, eigenvalues(matrix), want, 1e-8)
}

func TestEigenvaluesMatchAKnownDenseSpectrum(t *testing.T) {
	t.Parallel()

	// V diag(lambda) V^-1 for an explicitly inverted V, so the spectrum is
	// known while the matrix itself is dense and non-normal — the case the
	// Hessenberg reduction actually has to work for.
	//
	// V = [[1, 1], [1, 2]], V^-1 = [[2, -1], [-1, 1]].
	first, second := complex(0.9, 0.3), complex(-0.4, 0.8)

	matrix := [][]complex128{
		{2*first - second, -first + second},
		{2*first - 2*second, -first + 2*second},
	}

	assertSpectrum(t, eigenvalues(matrix), []complex128{first, second}, 1e-10)
}

// assertSpectrum compares two eigenvalue sets without assuming an order: QR
// deflation emits eigenvalues bottom-up, which is an implementation detail and
// not something a test should pin.
func assertSpectrum(t *testing.T, got, want []complex128, tolerance float64) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d eigenvalues, want %d", len(got), len(want))
	}

	matched := make([]bool, len(got))

	for _, expected := range want {
		found := false

		for index, actual := range got {
			if !matched[index] && cmplx.Abs(actual-expected) <= tolerance {
				matched[index], found = true, true

				break
			}
		}

		if !found {
			t.Errorf("eigenvalue %v is missing from %v", expected, got)
		}
	}
}

func TestSolveLeastSquaresReproducesAnExactSystem(t *testing.T) {
	t.Parallel()

	// Overdetermined but consistent: the least-squares solution is then the
	// exact one, so the answer is known rather than merely optimal.
	a := [][]complex128{
		{complex(1, 0), complex(0, 1)},
		{complex(2, -1), complex(1, 1)},
		{complex(0, 3), complex(-2, 0)},
		{complex(1, 1), complex(1, -1)},
	}

	want := [][]complex128{
		{complex(3, -1), complex(0, 2)},
		{complex(-2, 0.5), complex(1, 1)},
	}

	b := make([][]complex128, len(a))

	for row := range a {
		b[row] = make([]complex128, len(want[0]))
		for column := range want[0] {
			for k := range want {
				b[row][column] += a[row][k] * want[k][column]
			}
		}
	}

	got := solveLeastSquares(a, b)
	if got == nil {
		t.Fatal("solveLeastSquares reported a singular system")
	}

	for row := range want {
		for column := range want[row] {
			if cmplx.Abs(got[row][column]-want[row][column]) > 1e-9 {
				t.Errorf("solution[%d][%d] = %v, want %v",
					row, column, got[row][column], want[row][column])
			}
		}
	}
}

func TestSolveLeastSquaresMinimisesTheResidual(t *testing.T) {
	t.Parallel()

	// Inconsistent, so there is no exact answer and the defining property is
	// orthogonality: the residual must be perpendicular to every column of A.
	a := [][]complex128{
		{complex(1, 0), complex(0, 1)},
		{complex(1, 1), complex(2, 0)},
		{complex(0, -1), complex(1, 1)},
	}

	b := [][]complex128{
		{complex(1, 1)},
		{complex(0, -2)},
		{complex(3, 0)},
	}

	solution := solveLeastSquares(a, b)
	if solution == nil {
		t.Fatal("solveLeastSquares reported a singular system")
	}

	residual := make([]complex128, len(a))

	for row := range a {
		residual[row] = b[row][0]
		for k := range solution {
			residual[row] -= a[row][k] * solution[k][0]
		}
	}

	for column := range solution {
		var projection complex128
		for row := range a {
			projection += cmplx.Conj(a[row][column]) * residual[row]
		}

		if cmplx.Abs(projection) > 1e-9 {
			t.Errorf("residual is not orthogonal to column %d: %v", column, projection)
		}
	}
}

func TestSolveDenseReportsSingularity(t *testing.T) {
	t.Parallel()

	singular := [][]complex128{
		{complex(1, 0), complex(2, 0)},
		{complex(2, 0), complex(4, 0)},
	}

	right := [][]complex128{{complex(1, 0)}, {complex(1, 0)}}

	if solveDense(singular, right) != nil {
		t.Error("solveDense returned a solution for a singular matrix")
	}
}
