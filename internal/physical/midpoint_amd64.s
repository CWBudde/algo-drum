//go:build amd64 && !purego

#include "textflag.h"

// The batter head's implicit-midpoint update, four modes at a time.
//
// Per element, matching midpointReferenceBatter operation for operation:
//
//	nonlinear      = ratio * wave * wave
//	angularSquared = omegaSquared + nonlinear
//	denominator    = midpointDenom + 0.5*nonlinear*timeStep
//	numerator      = 2*velocity*inverseTimeStep - angularSquared*displacement
//	                 + forceN*strikeAccel + couplingAccel
//	stepDenominator  = denominator
//	midpointVelocity = numerator / denominator
//
// Bit-exact against the reference, and that is a requirement rather than a
// courtesy — the calibration fixture and the rendered-WAV digest both compare
// exactly. Two rules keep it so:
//
//   - No FMA. Go's compiler does not contract x*y+z on amd64, so each product is
//     rounded before it is added. VFMADD would skip that rounding.
//   - No reassociation. Operand order follows the Go source; only commutativity
//     of a single multiply or add is relied on, which IEEE 754 gives exactly.
//
// 2*velocity is formed as velocity+velocity, which is exact and saves a constant.
// 0.5*nonlinear is a multiply by the one constant this file defines rather than
// folding the half into timeStep, which would be a reassociation.

DATA midpointHalf<>+0(SB)/8, $0.5
GLOBL midpointHalf<>(SB), RODATA|NOPTR, $8

// func midpointBatterAVX2(n int, ratio, timeStep, inverseTimeStep, forceN float64,
//	wavenumber, omegaSquared, strikeAccel, midpointDenom []float64,
//	velocity, displacement, couplingAccel []float64,
//	stepDenominator, midpointVelocity []float64)
TEXT ·midpointBatterAVX2(SB), NOSPLIT|NOFRAME, $0-256
	MOVQ n+0(FP), R12
	TESTQ R12, R12
	JLE   done

	// Only the base pointers are needed; the caller has already established that
	// every slice covers n elements.
	MOVQ wavenumber_base+40(FP), BX
	MOVQ omegaSquared_base+64(FP), CX
	MOVQ strikeAccel_base+88(FP), DX
	MOVQ midpointDenom_base+112(FP), SI
	MOVQ velocity_base+136(FP), DI
	MOVQ displacement_base+160(FP), R8
	MOVQ couplingAccel_base+184(FP), R9
	MOVQ stepDenominator_base+208(FP), R10
	MOVQ midpointVelocity_base+232(FP), R11

	VBROADCASTSD ratio+8(FP), Y0
	VBROADCASTSD timeStep+16(FP), Y1
	VBROADCASTSD inverseTimeStep+24(FP), Y2
	VBROADCASTSD forceN+32(FP), Y3
	VBROADCASTSD midpointHalf<>(SB), Y4

	XORQ AX, AX

loop:
	// nonlinear = ratio * wave * wave
	VMOVUPD (BX)(AX*8), Y6
	VMULPD  Y0, Y6, Y7
	VMULPD  Y6, Y7, Y7

	// angularSquared = omegaSquared + nonlinear
	VADDPD (CX)(AX*8), Y7, Y8

	// denominator = midpointDenom + (0.5*nonlinear)*timeStep
	VMULPD Y4, Y7, Y9
	VMULPD Y1, Y9, Y9
	VADDPD (SI)(AX*8), Y9, Y9

	// numerator = (velocity+velocity)*inverseTimeStep
	VMOVUPD (DI)(AX*8), Y10
	VADDPD  Y10, Y10, Y10
	VMULPD  Y2, Y10, Y10

	// numerator -= angularSquared*displacement
	VMOVUPD (R8)(AX*8), Y11
	VMULPD  Y8, Y11, Y11
	VSUBPD  Y11, Y10, Y10

	// numerator += forceN*strikeAccel
	VMOVUPD (DX)(AX*8), Y11
	VMULPD  Y3, Y11, Y11
	VADDPD  Y11, Y10, Y10

	// numerator += couplingAccel
	VADDPD (R9)(AX*8), Y10, Y10

	VMOVUPD Y9, (R10)(AX*8)
	VDIVPD  Y9, Y10, Y10
	VMOVUPD Y10, (R11)(AX*8)

	ADDQ $4, AX
	CMPQ AX, R12
	JLT  loop

	VZEROUPPER

done:
	RET

// The resonant head's update: the same recurrence without either source term,
// since that head is never struck and the quartic table is batter-only.
//
//	nonlinear      = ratio * wave * wave
//	angularSquared = omegaSquared + nonlinear
//	denominator    = midpointDenom + 0.5*nonlinear*timeStep
//	numerator      = 2*velocity*inverseTimeStep - angularSquared*displacement
//	stepDenominator  = denominator
//	midpointVelocity = numerator / denominator
//
// Same two rules as above: no FMA, no reassociation.

// func midpointResonantAVX2(n int, ratio, timeStep, inverseTimeStep float64,
//	wavenumber, omegaSquared, midpointDenom []float64,
//	velocity, displacement []float64,
//	stepDenominator, midpointVelocity []float64)
TEXT ·midpointResonantAVX2(SB), NOSPLIT|NOFRAME, $0-200
	MOVQ  n+0(FP), R12
	TESTQ R12, R12
	JLE   resonantDone

	MOVQ wavenumber_base+32(FP), BX
	MOVQ omegaSquared_base+56(FP), CX
	MOVQ midpointDenom_base+80(FP), SI
	MOVQ velocity_base+104(FP), DI
	MOVQ displacement_base+128(FP), R8
	MOVQ stepDenominator_base+152(FP), R10
	MOVQ midpointVelocity_base+176(FP), R11

	VBROADCASTSD ratio+8(FP), Y0
	VBROADCASTSD timeStep+16(FP), Y1
	VBROADCASTSD inverseTimeStep+24(FP), Y2
	VBROADCASTSD midpointHalf<>(SB), Y4

	XORQ AX, AX

resonantLoop:
	// nonlinear = ratio * wave * wave
	VMOVUPD (BX)(AX*8), Y6
	VMULPD  Y0, Y6, Y7
	VMULPD  Y6, Y7, Y7

	// angularSquared = omegaSquared + nonlinear
	VADDPD (CX)(AX*8), Y7, Y8

	// denominator = midpointDenom + (0.5*nonlinear)*timeStep
	VMULPD Y4, Y7, Y9
	VMULPD Y1, Y9, Y9
	VADDPD (SI)(AX*8), Y9, Y9

	// numerator = (velocity+velocity)*inverseTimeStep
	VMOVUPD (DI)(AX*8), Y10
	VADDPD  Y10, Y10, Y10
	VMULPD  Y2, Y10, Y10

	// numerator -= angularSquared*displacement
	VMOVUPD (R8)(AX*8), Y11
	VMULPD  Y8, Y11, Y11
	VSUBPD  Y11, Y10, Y10

	VMOVUPD Y9, (R10)(AX*8)
	VDIVPD  Y9, Y10, Y10
	VMOVUPD Y10, (R11)(AX*8)

	ADDQ $4, AX
	CMPQ AX, R12
	JLT  resonantLoop

	VZEROUPPER

resonantDone:
	RET
