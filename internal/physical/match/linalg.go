package match

import (
	"math"
	"math/cmplx"
)

// The dense complex linear algebra subspace estimation needs, and nothing else.
//
// This repository has no linear-algebra dependency and acquiring one for four
// routines used by one offline estimator is the wrong trade: gonum's matrix
// tree is larger than this whole package, and the engine module is compiled for
// js/wasm, where every transitive dependency is shipped bytes. The matrices
// here are small by construction — the covariance is a few tens on a side and
// the shift-invariance matrix is single digits — so the naive O(n^3) forms
// below are not a compromise, they are the appropriate algorithms at this size.
//
// Everything is unexported and none of it runs inside a fit. Correctness is
// pinned against closed-form spectra in linalg_test.go rather than against
// another implementation, because there is no other implementation to compare
// against.

// hermitianEigen returns the eigenvalues of a Hermitian matrix in ascending
// order together with the matching orthonormal eigenvectors, vectors[k] being
// the eigenvector for values[k].
//
// Cyclic Jacobi rather than tridiagonal reduction plus QL: at these sizes the
// iteration count is irrelevant and Jacobi is the one form whose accuracy on
// the small eigenvalues does not depend on getting a reduction right. The small
// eigenvalues are the noise subspace, and ESTER reads its answer from exactly
// there.
//
// The matrix is not modified.
func hermitianEigen(matrix [][]complex128) (values []float64, vectors [][]complex128) {
	size := len(matrix)

	work := make([][]complex128, size)
	basis := make([][]complex128, size)

	for row := range size {
		work[row] = make([]complex128, size)
		copy(work[row], matrix[row])

		basis[row] = make([]complex128, size)
		basis[row][row] = 1
	}

	// Each sweep visits every off-diagonal pair once. Jacobi converges
	// quadratically, so the cap is a runaway guard rather than a working limit;
	// a converged sweep exits below.
	const maxSweeps = 60

	for range maxSweeps {
		off := 0.0

		for row := range size {
			for column := row + 1; column < size; column++ {
				magnitude := cmplx.Abs(work[row][column])
				off += magnitude * magnitude
			}
		}

		if off <= 1e-30 {
			break
		}

		for pivot := range size {
			for other := pivot + 1; other < size; other++ {
				jacobiRotate(work, basis, pivot, other)
			}
		}
	}

	values = make([]float64, size)
	for index := range size {
		values[index] = real(work[index][index])
	}

	// The eigenvectors are the columns of the accumulated rotation; the caller
	// wants them one per slice, so they are transposed out here.
	vectors = make([][]complex128, size)

	for index := range size {
		vectors[index] = make([]complex128, size)
		for row := range size {
			vectors[index][row] = basis[row][index]
		}
	}

	sortEigenpairs(values, vectors)

	return values, vectors
}

// jacobiRotate annihilates work[pivot][other] with one unitary similarity and
// accumulates it into basis.
//
// A complex Hermitian pair is diagonalised in two steps rather than one: a
// diagonal phase makes the off-diagonal entry real, and then the textbook real
// symmetric Jacobi rotation applies unchanged. The composite V = D R is what is
// applied, so no intermediate matrix is ever formed.
func jacobiRotate(work, basis [][]complex128, pivot, other int) {
	entry := work[pivot][other]

	magnitude := cmplx.Abs(entry)
	if magnitude <= 1e-300 {
		return
	}

	diagonalPivot := real(work[pivot][pivot])
	diagonalOther := real(work[other][other])

	// The rotation angle, in the standard numerically-safe form: tau is large
	// when the diagonal is already dominant, and writing t this way keeps the
	// small root rather than losing it to cancellation.
	tau := (diagonalOther - diagonalPivot) / (2 * magnitude)

	tangent := 1 / (math.Abs(tau) + math.Sqrt(1+tau*tau))
	if tau < 0 {
		tangent = -tangent
	}

	cosine := 1 / math.Sqrt(1+tangent*tangent)
	sine := tangent * cosine

	phase := entry / complex(magnitude, 0) // e^{i·arg}
	conjugatePhase := cmplx.Conj(phase)

	// V has V[p][p] = c, V[p][q] = s, V[q][p] = -s·conj(phase),
	// V[q][q] = c·conj(phase). Columns first, then rows, which is A ← V^H A V.
	size := len(work)

	for row := range size {
		left, right := work[row][pivot], work[row][other]
		work[row][pivot] = complex(cosine, 0)*left - complex(sine, 0)*conjugatePhase*right
		work[row][other] = complex(sine, 0)*left + complex(cosine, 0)*conjugatePhase*right
	}

	for column := range size {
		upper, lower := work[pivot][column], work[other][column]
		work[pivot][column] = complex(cosine, 0)*upper - complex(sine, 0)*phase*lower
		work[other][column] = complex(sine, 0)*upper + complex(cosine, 0)*phase*lower
	}

	// The two off-diagonal entries are zero by construction; assigning them
	// rather than trusting the arithmetic keeps the convergence test exact.
	work[pivot][other], work[other][pivot] = 0, 0

	for row := range size {
		left, right := basis[row][pivot], basis[row][other]
		basis[row][pivot] = complex(cosine, 0)*left - complex(sine, 0)*conjugatePhase*right
		basis[row][other] = complex(sine, 0)*left + complex(cosine, 0)*conjugatePhase*right
	}
}

// sortEigenpairs orders values ascending, carrying vectors along.
func sortEigenpairs(values []float64, vectors [][]complex128) {
	for index := 1; index < len(values); index++ {
		for back := index; back > 0 && values[back] < values[back-1]; back-- {
			values[back], values[back-1] = values[back-1], values[back]
			vectors[back], vectors[back-1] = vectors[back-1], vectors[back]
		}
	}
}

// eigenvalues returns the eigenvalues of a general square complex matrix, by
// Householder reduction to upper Hessenberg form followed by shifted QR.
//
// The complex case is the easy one: a real unsymmetric matrix needs 2x2 blocks
// and Francis double shifts to keep conjugate pairs together in real
// arithmetic, and none of that applies here. Every deflation produces one
// eigenvalue.
//
// The matrix is not modified.
func eigenvalues(matrix [][]complex128) []complex128 {
	size := len(matrix)
	if size == 0 {
		return nil
	}

	work := make([][]complex128, size)
	for row := range size {
		work[row] = make([]complex128, size)
		copy(work[row], matrix[row])
	}

	toHessenberg(work)

	return hessenbergQR(work)
}

// toHessenberg reduces a square complex matrix to upper Hessenberg form in
// place by Householder similarity.
//
// The reflector I - 2vv^H is both Hermitian and unitary, so its inverse is
// itself and the similarity is a left multiply followed by a right multiply by
// the same matrix — there is no separate inverse to form.
func toHessenberg(work [][]complex128) {
	size := len(work)

	reflector := make([]complex128, size)

	for column := 0; column+2 < size; column++ {
		norm := 0.0

		for row := column + 1; row < size; row++ {
			magnitude := cmplx.Abs(work[row][column])
			norm += magnitude * magnitude
		}

		norm = math.Sqrt(norm)
		if norm <= 1e-300 {
			continue
		}

		head := work[column+1][column]

		// Reflect away from the leading entry rather than towards it, which is
		// the difference between subtracting two nearly equal numbers and not.
		alpha := complex(-norm, 0)
		if magnitude := cmplx.Abs(head); magnitude > 0 {
			alpha = -head / complex(magnitude, 0) * complex(norm, 0)
		}

		for row := column + 1; row < size; row++ {
			reflector[row] = work[row][column]
		}

		reflector[column+1] -= alpha

		scale := 0.0

		for row := column + 1; row < size; row++ {
			magnitude := cmplx.Abs(reflector[row])
			scale += magnitude * magnitude
		}

		if scale <= 1e-300 {
			continue
		}

		scale = math.Sqrt(scale)
		for row := column + 1; row < size; row++ {
			reflector[row] /= complex(scale, 0)
		}

		// A ← (I - 2vv^H) A
		for target := range size {
			var projection complex128
			for row := column + 1; row < size; row++ {
				projection += cmplx.Conj(reflector[row]) * work[row][target]
			}

			projection *= 2
			for row := column + 1; row < size; row++ {
				work[row][target] -= projection * reflector[row]
			}
		}

		// A ← A (I - 2vv^H)
		for target := range size {
			var projection complex128
			for row := column + 1; row < size; row++ {
				projection += work[target][row] * reflector[row]
			}

			projection *= 2
			for row := column + 1; row < size; row++ {
				work[target][row] -= projection * cmplx.Conj(reflector[row])
			}
		}
	}
}

// hessenbergQR runs shifted QR on an upper Hessenberg matrix until every
// eigenvalue has deflated off the bottom.
func hessenbergQR(work [][]complex128) []complex128 {
	size := len(work)
	found := make([]complex128, size)

	// Iterations are counted across the whole matrix rather than per
	// eigenvalue, so a matrix that will not converge costs a bounded amount
	// once instead of once per deflation.
	const maxIterations = 100

	high := size - 1
	iterations := 0

	for high >= 0 {
		if high == 0 {
			found[0] = work[0][0]
			break
		}

		low := findDeflation(work, high)

		if low == high {
			found[high] = work[high][high]
			high--
			iterations = 0

			continue
		}

		if iterations >= maxIterations {
			// Give up on this block rather than spin: report its diagonal,
			// which for a nearly-converged Hessenberg block is close, and let
			// the caller's own plausibility filters deal with it.
			for index := low; index <= high; index++ {
				found[index] = work[index][index]
			}

			high = low - 1
			iterations = 0

			continue
		}

		shift := wilkinsonShift(work, high)

		// Every fifth iteration, shift by something unrelated to the matrix.
		// The classic failure of a shifted QR is a block whose own eigenvalue
		// estimate is exactly the symmetric configuration that will not break,
		// and an arbitrary shift is what breaks it.
		if iterations > 0 && iterations%5 == 0 {
			shift = work[high][high] + complex(0.75*cmplx.Abs(work[high][high-1]), 0)
		}

		qrStep(work, low, high, shift)

		iterations++
	}

	return found
}

// findDeflation returns the top row of the trailing block that still has to be
// iterated on, zeroing any subdiagonal entry that is negligible against its
// neighbouring diagonal entries.
func findDeflation(work [][]complex128, high int) int {
	for row := high; row > 0; row-- {
		scale := cmplx.Abs(work[row-1][row-1]) + cmplx.Abs(work[row][row])
		if scale == 0 {
			scale = 1
		}

		if cmplx.Abs(work[row][row-1]) <= 1e-15*scale {
			work[row][row-1] = 0

			return row
		}
	}

	return 0
}

// wilkinsonShift is the eigenvalue of the trailing 2x2 block nearer to the
// bottom-right entry, which is the shift that gives QR its cubic convergence.
func wilkinsonShift(work [][]complex128, high int) complex128 {
	upperLeft := work[high-1][high-1]
	upperRight := work[high-1][high]
	lowerLeft := work[high][high-1]
	lowerRight := work[high][high]

	half := (upperLeft - lowerRight) / 2

	root := cmplx.Sqrt(half*half + upperRight*lowerLeft)

	first, second := lowerRight+half+root, lowerRight+half-root
	if cmplx.Abs(first-lowerRight) <= cmplx.Abs(second-lowerRight) {
		return first
	}

	return second
}

// qrStep performs one explicitly shifted QR sweep on rows low..high: the block
// is factored with Givens rotations and the factors are remultiplied in the
// opposite order, which is a unitary similarity and so leaves the spectrum
// alone.
func qrStep(work [][]complex128, low, high int, shift complex128) {
	size := len(work)

	for index := low; index <= high; index++ {
		work[index][index] -= shift
	}

	cosines := make([]float64, high)
	sines := make([]complex128, high)

	for index := low; index < high; index++ {
		cosine, sine := givens(work[index][index], work[index+1][index])
		cosines[index], sines[index] = cosine, sine

		applyGivensLeft(work, index, cosine, sine, index, size-1)
	}

	for index := low; index < high; index++ {
		applyGivensRight(work, index, cosines[index], sines[index], 0, min(index+2, high))
	}

	for index := low; index <= high; index++ {
		work[index][index] += shift
	}
}

// givens returns the rotation that annihilates lower against upper. The cosine
// is real and the sine complex, which is the standard complex Givens form.
func givens(upper, lower complex128) (cosine float64, sine complex128) {
	scale := math.Hypot(cmplx.Abs(upper), cmplx.Abs(lower))
	if scale == 0 {
		return 1, 0
	}

	return cmplx.Abs(upper) / scale, lower / complex(scale, 0) * phaseOf(upper)
}

// phaseOf is the unit complex number with the same argument as value, and 1 for
// a value with no argument to speak of.
func phaseOf(value complex128) complex128 {
	magnitude := cmplx.Abs(value)
	if magnitude <= 1e-300 {
		return 1
	}

	return cmplx.Conj(value) / complex(magnitude, 0)
}

// applyGivensLeft multiplies rows index and index+1 by the rotation, over the
// column span given.
func applyGivensLeft(work [][]complex128, index int, cosine float64, sine complex128, from, to int) {
	for column := from; column <= to; column++ {
		upper, lower := work[index][column], work[index+1][column]
		work[index][column] = complex(cosine, 0)*upper + cmplx.Conj(sine)*lower
		work[index+1][column] = -sine*upper + complex(cosine, 0)*lower
	}
}

// applyGivensRight multiplies columns index and index+1 by the rotation's
// adjoint, over the row span given.
func applyGivensRight(work [][]complex128, index int, cosine float64, sine complex128, from, to int) {
	for row := from; row <= to; row++ {
		left, right := work[row][index], work[row][index+1]
		work[row][index] = complex(cosine, 0)*left + sine*right
		work[row][index+1] = -cmplx.Conj(sine)*left + complex(cosine, 0)*right
	}
}

// solveLeastSquares returns the X minimising ||A X - B|| in the Frobenius norm,
// via the normal equations A^H A X = A^H B.
//
// Normal equations square the condition number, which is normally the reason
// not to use them. Here A is a block of orthonormal subspace basis vectors with
// one row removed, so its conditioning is set by how far that removal is from
// unitary rather than by the data, and the squaring costs nothing measurable.
// Both callers below pass such a block.
//
// Returns nil if the system is singular.
func solveLeastSquares(matrix, target [][]complex128) [][]complex128 {
	rows, columns := len(matrix), len(matrix[0])
	targets := len(target[0])

	normal := make([][]complex128, columns)
	rightHand := make([][]complex128, columns)

	for row := range columns {
		normal[row] = make([]complex128, columns)
		rightHand[row] = make([]complex128, targets)

		for column := range columns {
			var sum complex128
			for k := range rows {
				sum += cmplx.Conj(matrix[k][row]) * matrix[k][column]
			}

			normal[row][column] = sum
		}

		for column := range targets {
			var sum complex128
			for k := range rows {
				sum += cmplx.Conj(matrix[k][row]) * target[k][column]
			}

			rightHand[row][column] = sum
		}
	}

	return solveDense(normal, rightHand)
}

// solveDense solves A X = B by Gaussian elimination with partial pivoting,
// returning nil if A is singular. Both arguments are modified.
func solveDense(matrix, target [][]complex128) [][]complex128 {
	size := len(matrix)
	targets := len(target[0])

	for column := range size {
		pivot := column
		for row := column + 1; row < size; row++ {
			if cmplx.Abs(matrix[row][column]) > cmplx.Abs(matrix[pivot][column]) {
				pivot = row
			}
		}

		if cmplx.Abs(matrix[pivot][column]) <= 1e-300 {
			return nil
		}

		matrix[column], matrix[pivot] = matrix[pivot], matrix[column]
		target[column], target[pivot] = target[pivot], target[column]

		for row := column + 1; row < size; row++ {
			factor := matrix[row][column] / matrix[column][column]
			if factor == 0 {
				continue
			}

			for k := column; k < size; k++ {
				matrix[row][k] -= factor * matrix[column][k]
			}

			for k := range targets {
				target[row][k] -= factor * target[column][k]
			}
		}
	}

	solution := make([][]complex128, size)
	for row := range size {
		solution[row] = make([]complex128, targets)
	}

	for row := size - 1; row >= 0; row-- {
		for column := range targets {
			sum := target[row][column]
			for inner := row + 1; inner < size; inner++ {
				sum -= matrix[row][inner] * solution[inner][column]
			}

			solution[row][column] = sum / matrix[row][row]
		}
	}

	return solution
}
