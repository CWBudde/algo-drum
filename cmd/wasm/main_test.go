//go:build js && wasm

package main

import (
	"math"
	"reflect"
	"syscall/js"
	"testing"

	"github.com/cwbudde/algo-drum/internal/drum"
)

func TestAPIRequiresInit(t *testing.T) {
	api := newTestAPI(t)

	nullMethods := []string{
		"setRunning", "beginStart", "pause", "setTempo", "setSwing", "setStepCount",
		"setCell", "setCellProbability", "setCellHumanize", "setCellCondition",
		"setCellRepeats", "setTrackLength", "setFillMode", "setMetronomeEnabled",
		"setCountInBars", "setPattern", "setPatternBank", "requestBank", "setChain",
		"setChainEnabled", "setState", "getState", "setVolume", "setMuted", "setDecay",
		"setVoiceParam", "setPhysicalTomParam", "setTomModel", "triggerVoice", "setReverb",
		"setProbability", "setHumanize",
	}
	for _, name := range nullMethods {
		if got := api.Call(name); got.Type() != js.TypeNull {
			t.Errorf("%s() before init returned %s, want null", name, got.Type())
		}
	}

	rendered := api.Call("render", 512)
	assertInstanceOf(t, rendered, "Float32Array")
	if got := rendered.Length(); got != 0 {
		t.Errorf("render() before init length = %d, want 0", got)
	}

	if got := api.Call("currentStep").Int(); got != -1 {
		t.Errorf("currentStep() before init = %d, want -1", got)
	}
	if got := api.Call("transportState").String(); got != string(drum.TransportStopped) {
		t.Errorf("transportState() before init = %q, want %q", got, drum.TransportStopped)
	}
	if got := api.Call("transportRevision").Int(); got != 0 {
		t.Errorf("transportRevision() before init = %d, want 0", got)
	}
	if got := api.Call("activeBank").Int(); got != 0 {
		t.Errorf("activeBank() before init = %d, want 0", got)
	}
	if got := api.Call("queuedBank").Int(); got != drum.NoBank {
		t.Errorf("queuedBank() before init = %d, want %d", got, drum.NoBank)
	}
	if got := api.Call("chainPosition").Int(); got != -1 {
		t.Errorf("chainPosition() before init = %d, want -1", got)
	}
	if !api.Call("isIdle").Bool() {
		t.Error("isIdle() before init = false, want true")
	}
	if got := api.Get("protocolVersion").Int(); got != drum.ProtocolVersion {
		t.Errorf("protocolVersion = %d, want %d", got, drum.ProtocolVersion)
	}
	if engine != nil {
		t.Fatal("an API call before init created an engine")
	}
	if !warnedBeforeInit {
		t.Error("API calls before init did not trip the warning gate")
	}
	if warnedBadArg {
		t.Error("pre-init calls parsed arguments before checking readiness")
	}
}

func TestArgumentMarshallingAndClamping(t *testing.T) {
	api := newTestAPI(t)

	floatTests := []struct {
		name  string
		value js.Value
		want  float64
		ok    bool
	}{
		{name: "integer", value: js.ValueOf(12), want: 12, ok: true},
		{name: "fraction", value: js.ValueOf(12.5), want: 12.5, ok: true},
		{name: "string", value: js.ValueOf("12"), ok: false},
		{name: "boolean", value: js.ValueOf(true), ok: false},
		{name: "null", value: js.Null(), ok: false},
		{name: "undefined", value: js.Undefined(), ok: false},
		{name: "nan", value: js.ValueOf(math.NaN()), ok: false},
		{name: "positive infinity", value: js.ValueOf(math.Inf(1)), ok: false},
		{name: "negative infinity", value: js.ValueOf(math.Inf(-1)), ok: false},
	}
	for _, test := range floatTests {
		t.Run("float/"+test.name, func(t *testing.T) {
			got, ok := argFloat([]js.Value{test.value}, 0, "test")
			if ok != test.ok || ok && got != test.want {
				t.Errorf("argFloat() = (%v, %v), want (%v, %v)", got, ok, test.want, test.ok)
			}
		})
	}
	if _, ok := argFloat(nil, 0, "test"); ok {
		t.Error("argFloat accepted a missing argument")
	}

	intTests := []struct {
		name  string
		value float64
		want  int
		ok    bool
	}{
		{name: "minimum", value: math.MinInt32, want: math.MinInt32, ok: true},
		{name: "maximum", value: math.MaxInt32, want: math.MaxInt32, ok: true},
		{name: "fraction", value: 7.9, ok: false},
		{name: "below minimum", value: math.MinInt32 - 1, ok: false},
		{name: "above maximum", value: math.MaxInt32 + 1, ok: false},
	}
	for _, test := range intTests {
		t.Run("int/"+test.name, func(t *testing.T) {
			got, ok := argInt([]js.Value{js.ValueOf(test.value)}, 0, "test")
			if ok != test.ok || ok && got != test.want {
				t.Errorf("argInt() = (%d, %v), want (%d, %v)", got, ok, test.want, test.ok)
			}
		})
	}

	for _, test := range []struct {
		name  string
		value js.Value
		want  bool
		ok    bool
	}{
		{name: "true", value: js.ValueOf(true), want: true, ok: true},
		{name: "false", value: js.ValueOf(false), want: false, ok: true},
		{name: "number", value: js.ValueOf(1), ok: false},
		{name: "string", value: js.ValueOf("true"), ok: false},
	} {
		t.Run("bool/"+test.name, func(t *testing.T) {
			got, ok := argBool([]js.Value{test.value}, 0, "test")
			if ok != test.ok || ok && got != test.want {
				t.Errorf("argBool() = (%v, %v), want (%v, %v)", got, ok, test.want, test.ok)
			}
		})
	}

	api.Call("init", 48000)
	api.Call("setTempo", 999)
	if got := engine.State().TempoBPM; got != 300 {
		t.Errorf("setTempo(999) stored %v, want clamp to 300", got)
	}

	want := engine.State()
	for _, bad := range []any{"120", math.NaN(), math.Inf(1)} {
		api.Call("setTempo", bad)
	}
	if got := engine.State(); !reflect.DeepEqual(got, want) {
		t.Error("invalid setTempo argument mutated engine state")
	}

	api.Call("setCell", 0, 0, 0, 2)
	if got := engine.State().Banks[0].Pattern[0]; got != 1 {
		t.Errorf("setCell velocity stored %v, want clamp to 1", got)
	}
	want = engine.State()
	api.Call("setCell", 0, 0.5, 0, 0.25)
	if got := engine.State(); !reflect.DeepEqual(got, want) {
		t.Error("fractional track index mutated engine state")
	}

	api.Call("setRunning", "true")
	if got := api.Call("transportState").String(); got != string(drum.TransportStopped) {
		t.Errorf("wrong-typed setRunning changed transport to %q", got)
	}
}

func TestBulkTypedArraysAndStateRoundTrip(t *testing.T) {
	api := newTestAPI(t)
	api.Call("init", 48000)

	float32Array := js.Global().Get("Float32Array")
	pattern := float32Array.New(drum.PatternSize)
	pattern.SetIndex(0, 0.25)
	pattern.SetIndex(drum.PatternSize-1, 2)
	api.Call("setPattern", 1, pattern)
	gotState := engine.State()
	if got := gotState.Banks[1].Pattern[0]; got != 0.25 {
		t.Errorf("typed pattern[0] = %v, want 0.25", got)
	}
	if got := gotState.Banks[1].Pattern[drum.PatternSize-1]; got != 1 {
		t.Errorf("typed pattern[last] = %v, want clamp to 1", got)
	}

	plainPattern := js.Global().Get("Array").New(drum.PatternSize)
	for i := range drum.PatternSize {
		plainPattern.SetIndex(i, 0)
	}
	plainPattern.SetIndex(17, 0.5)
	api.Call("setPattern", 2, plainPattern)
	if got := engine.State().Banks[2].Pattern[17]; got != 0.5 {
		t.Errorf("plain pattern[17] = %v, want 0.5", got)
	}

	for name, bad := range map[string]js.Value{
		"short":      float32Array.New(drum.PatternSize - 1),
		"no length":  js.Global().Get("Object").New(),
		"not object": js.ValueOf("pattern"),
	} {
		t.Run("reject pattern/"+name, func(t *testing.T) {
			want := engine.State()
			api.Call("setPattern", 0, bad)
			if got := engine.State(); !reflect.DeepEqual(got, want) {
				t.Error("malformed pattern mutated engine state")
			}
		})
	}
	badNumber := float32Array.New(drum.PatternSize)
	badNumber.SetIndex(3, math.NaN())
	want := engine.State()
	api.Call("setPattern", 0, badNumber)
	if got := engine.State(); !reflect.DeepEqual(got, want) {
		t.Error("non-finite pattern mutated engine state")
	}

	chain := js.Global().Get("Uint8Array").New(3)
	chain.SetIndex(0, 2)
	chain.SetIndex(1, 0)
	chain.SetIndex(2, 3)
	api.Call("setChain", chain)
	if got := engine.State().Chain; !reflect.DeepEqual(got, []int{2, 0, 3}) {
		t.Errorf("setChain() stored %v, want [2 0 3]", got)
	}

	out := api.Call("getState")
	assertStateOutputTypes(t, out)
	out.Set("tempoBpm", 177.5)
	out.Get("banks").Index(0).Get("pattern").SetIndex(5, 0.75)
	api.Call("setState", out)
	gotState = engine.State()
	if gotState.TempoBPM != 177.5 || gotState.Banks[0].Pattern[5] != 0.75 {
		t.Errorf("state round trip stored tempo/pattern = %v/%v", gotState.TempoBPM, gotState.Banks[0].Pattern[5])
	}

	bank := api.Call("getState").Get("banks").Index(0)
	bank.Set("stepCount", 9)
	bank.Get("cellRepeats").SetIndex(5, 3)
	api.Call("setPatternBank", 3, bank)
	gotState = engine.State()
	if gotState.Banks[3].StepCount != 9 || gotState.Banks[3].CellRepeats[5] != 3 {
		t.Errorf("pattern-bank round trip stored step count/repeat = %d/%d", gotState.Banks[3].StepCount, gotState.Banks[3].CellRepeats[5])
	}

	badState := api.Call("getState")
	want = engine.State()
	badState.Get("banks").Index(0).Get("cellConditions").SetIndex(0, 99)
	api.Call("setState", badState)
	if got := engine.State(); !reflect.DeepEqual(got, want) {
		t.Error("structurally invalid setState input partially mutated engine")
	}

	badBank := api.Call("getState").Get("banks").Index(0)
	want = engine.State()
	badBank.Get("cellRepeats").SetIndex(0, 0)
	api.Call("setPatternBank", 0, badBank)
	if got := engine.State(); !reflect.DeepEqual(got, want) {
		t.Error("structurally invalid setPatternBank input partially mutated engine")
	}

	firstState := api.Call("getState")
	firstPattern := firstState.Get("banks").Index(0).Get("pattern")
	api.Call("setCell", 0, 0, 0, 0.375)
	secondState := api.Call("getState")
	secondPattern := secondState.Get("banks").Index(0).Get("pattern")
	if !firstState.Equal(secondState) || !firstPattern.Equal(secondPattern) {
		t.Error("getState did not reuse its state object and fixed-size typed arrays")
	}
	if got := firstPattern.Index(0).Float(); got != float64(float32(0.375)) {
		t.Errorf("reused pattern view = %v, want %v", got, float64(float32(0.375)))
	}
}

func TestRenderCapsCopiesAndReusesBuffers(t *testing.T) {
	api := newTestAPI(t)
	api.Call("init", 48000)

	for name, arg := range map[string]any{
		"zero":       0,
		"negative":   -1,
		"fractional": 1.5,
		"string":     "512",
		"infinite":   math.Inf(1),
	} {
		t.Run("empty/"+name, func(t *testing.T) {
			got := api.Call("render", arg)
			assertInstanceOf(t, got, "Float32Array")
			if got.Length() != 0 {
				t.Errorf("render(%v) length = %d, want 0", arg, got.Length())
			}
		})
	}

	small := api.Call("render", 8)
	assertInstanceOf(t, small, "Float32Array")
	if got := small.Length(); got != 8 {
		t.Fatalf("small render length = %d, want 8", got)
	}
	smallBuffer := small.Get("buffer")

	api.Call("triggerVoice", 0, 1)
	large := api.Call("render", 4096)
	if got := large.Length(); got != 4096 {
		t.Fatalf("large render length = %d, want 4096", got)
	}
	largeBuffer := large.Get("buffer")
	if smallBuffer.Equal(largeBuffer) {
		t.Error("growing the render buffer reused undersized JS storage")
	}
	if !largeBuffer.Equal(jsFloats.Get("buffer")) || !largeBuffer.Equal(jsBytes.Get("buffer")) {
		t.Error("render Float32Array and Uint8Array do not share the same ArrayBuffer")
	}

	nonzero := false
	for i, want := range renderBuf[:large.Length()] {
		got := large.Index(i).Float()
		if got != float64(want) {
			t.Fatalf("rendered sample %d = %v, want float32 copy %v", i, got, want)
		}
		if want != 0 {
			nonzero = true
		}
	}
	if !nonzero {
		t.Error("triggered render contained no audio, so its unsafe copy was not meaningfully tested")
	}

	shrunk := api.Call("render", 64)
	if got := shrunk.Length(); got != 64 {
		t.Errorf("shrunk render length = %d, want 64", got)
	}
	if !shrunk.Get("buffer").Equal(largeBuffer) {
		t.Error("smaller render did not reuse existing JS storage")
	}

	api.Call("init", 48000)
	capped := api.Call("render", maxRenderSamples+1)
	if got := capped.Length(); got != maxRenderSamples {
		t.Errorf("oversized render length = %d, want cap %d", got, maxRenderSamples)
	}
	if capped.Get("byteOffset").Int() != 0 {
		t.Errorf("capped render byteOffset = %d, want 0", capped.Get("byteOffset").Int())
	}

	values, bytes, floats := newFloatOutput(4)
	copyFloatOutput(values, bytes, []float64{-0.5, 0.1, 0.25, 1})
	for i, want := range values {
		if got := floats.Index(i).Float(); got != float64(want) {
			t.Errorf("copyFloatOutput[%d] = %v, want %v", i, got, want)
		}
	}
	emptyValues, emptyBytes, _ := newFloatOutput(0)
	copyFloatOutput(emptyValues, emptyBytes, nil)
}

func newTestAPI(t *testing.T) js.Value {
	t.Helper()
	resetWASMState()
	api := newAPI()
	t.Cleanup(func() {
		for _, fn := range funcs {
			fn.Release()
		}
		funcs = nil
		resetWASMState()
	})

	return api
}

func resetWASMState() {
	engine = nil
	warnedBeforeInit = false
	warnedBadArg = false
	renderBuf = nil
	jsBytes = js.Undefined()
	jsFloats = js.Undefined()
	patternInput = [drum.PatternSize]float64{}
	stateOut = stateOutput{}
}

func assertInstanceOf(t *testing.T, value js.Value, constructor string) {
	t.Helper()
	if !value.InstanceOf(js.Global().Get(constructor)) {
		t.Errorf("value is not a %s", constructor)
	}
}

func assertStateOutputTypes(t *testing.T, state js.Value) {
	t.Helper()
	array := js.Global().Get("Array")
	if !array.Call("isArray", state.Get("banks")).Bool() {
		t.Error("state.banks is not an Array")
	}
	if !array.Call("isArray", state.Get("tracks")).Bool() {
		t.Error("state.tracks is not an Array")
	}
	assertInstanceOf(t, state.Get("chain"), "Uint8Array")

	for bank := range drum.PatternBankCount {
		value := state.Get("banks").Index(bank)
		for _, name := range []string{"pattern", "cellProbabilities", "cellHumanize"} {
			assertInstanceOf(t, value.Get(name), "Float32Array")
		}
		for _, name := range []string{"cellConditions", "cellRepeats", "trackLengths"} {
			assertInstanceOf(t, value.Get(name), "Uint8Array")
		}
	}

	for track := range drum.TrackCount {
		value := state.Get("tracks").Index(track)
		assertInstanceOf(t, value.Get("voiceParams"), "Float32Array")
		if track == 3 || track == 5 {
			assertInstanceOf(t, value.Get("tom").Get("physicalParams"), "Float32Array")
		}
	}
}
