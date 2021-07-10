package checkbms

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestScanBmsFile(t *testing.T) {

}

func TestCheckHeaderCommands(t *testing.T) {
	allOkHeader := map[string]string{
		"player":     "1",
		"genre":      "test",
		"title":      "test",
		"artist":     "test",
		"bpm":        "100",
		"playlevel":  "1",
		"rank":       "2",
		"total":      "100",
		"difficulty": "1",
	}
	copyAllOkHeader := func(m map[string]string) {
		for key, value := range allOkHeader {
			m[key] = value
		}
	}

	type Test struct {
		name    string
		bmsFile *BmsFile
		want    string
	}

	tests := []Test{
		{
			name: "all OK",
			bmsFile: func() *BmsFile {
				bf := &BmsFile{Header: map[string]string{}}
				copyAllOkHeader(bf.Header)
				return bf
			}(),
			want: "",
		},
		{
			name:    "all missing",
			bmsFile: &BmsFile{Header: map[string]string{}},
			want: func() string {
				str := ""
				for _, command := range COMMANDS {
					if command.Necessity != Unnecessary {
						level := Error
						if command.Necessity == Semi_necessary {
							level = Warning
						}
						str += fmt.Sprintf("%s: #%s definition is missing\n", string(level), strings.ToUpper(command.Name))
					}
				}
				if len(str) > 0 {
					str = str[:len(str)-1] // 最後の改行を削る
				}
				return str
			}(),
		},
		{
			name: "all empty",
			bmsFile: func() *BmsFile {
				bf := &BmsFile{Header: map[string]string{}}
				for _, command := range COMMANDS {
					bf.Header[command.Name] = ""
				}
				return bf
			}(),
			want: func() string {
				str := ""
				for i, command := range COMMANDS {
					str += fmt.Sprintf("WARNING: #%s value is empty", strings.ToUpper(command.Name))
					if i < len(COMMANDS)-1 {
						str += "\n"
					}
				}
				return str
			}(),
		},
	}

	type headerValue struct{ command, value string }
	newOkTest := func(name string, hs []headerValue) *Test {
		return &Test{
			name: name,
			bmsFile: func() *BmsFile {
				bf := &BmsFile{Header: map[string]string{}}
				copyAllOkHeader(bf.Header)
				for _, s := range hs {
					bf.Header[s.command] = s.value
				}
				return bf
			}(),
			want: "",
		}
	}
	newInvalidTest := func(name string, hs []headerValue) *Test {
		t := newOkTest(name, hs)
		t.want = func() string {
			str := ""
			for _, s := range hs {
				str += fmt.Sprintf("ERROR: #%s has invalid value: %s\n", strings.ToUpper(s.command), s.value)
			}
			if len(str) > 0 {
				str = str[:len(str)-1] // 最後の改行を削る
			}
			return str
		}()
		return t
	}

	strMaxInt := strconv.Itoa(math.MaxInt64)
	strMaxFloat := strconv.FormatFloat(math.MaxFloat64, 'f', -1, 64)
	strMinFloat := strconv.FormatFloat(math.SmallestNonzeroFloat64, 'f', -1, 64)
	tests = append(tests, func() Test {
		t := newOkTest("max value", []headerValue{
			{"player", "4"},
			{"bpm", strMaxFloat},
			{"playlevel", strMaxInt},
			{"rank", "4"},
			{"defexrank", strMaxFloat},
			{"total", strMaxFloat},
			{"difficulty", "5"},
			{"lntype", "2"},
			{"lnmode", "3"},
			{"volwav", "0"},
		})
		t.want = "NOTICE: #RANK is 4(VERY EASY)\n" +
			fmt.Sprintf("NOTICE: #DEFEXRANK is defined: %s\n", strMaxFloat) +
			"WARNING: #LNTYPE 2(MGQ) is deprecated"
		return *t
	}())
	tests = append(tests, func() Test {
		t := newOkTest("min value", []headerValue{
			{"player", "1"},
			{"bpm", strMinFloat},
			{"playlevel", "0"},
			{"rank", "0"},
			{"defexrank", "0.0"},
			{"total", "0.0"},
			{"difficulty", "0"},
			{"lntype", "1"},
			{"lnmode", "1"},
			{"volwav", "0"},
		})
		t.want = "NOTICE: #RANK is 0(VERY HARD)\n" +
			"NOTICE: #DEFEXRANK is defined: 0.0\n" +
			"WARNING: #TOTAL is under 100: 0.0\n" +
			"WARNING: #DIFFICULTY is 0(Undefined)"
		return *t
	}())
	tests = append(tests, *newOkTest("ok path", []headerValue{
		{"stagefile", "test.jpg"},
		{"banner", "1234.PNG"},
		{"backbmp", "画像.BmP"},
		{"preview", "test.wav"},
	}))
	tests = append(tests, *newInvalidTest("invalid type", []headerValue{
		{"player", "1.0"},
		{"bpm", "test"},
		{"playlevel", "あいうえお"},
		{"rank", "-100"},
		{"difficulty", "6"},
		{"lnobj", "AAA"},
	}))
	tests = append(tests, *newInvalidTest("invalid over", []headerValue{
		{"player", "5"},
		{"rank", "5"},
		{"difficulty", "6"},
		{"lntype", "3"},
		{"lnmode", "4"},
	}))
	tests = append(tests, *newInvalidTest("invalid under", []headerValue{
		{"player", "0"},
		{"playlevel", "-1"},
		{"rank", "-1"},
		{"difficulty", "-1"},
		{"lntype", "0"},
		{"lnmode", "0"},
		{"volwav", "-1"},
	}))
	tests = append(tests, *newInvalidTest("invalid path", []headerValue{
		{"stagefile", "jpg"},
		{"banner", "100"},
		{"backbmp", "test.wav"},
		{"preview", "test.jpg"},
	}))

	tests = append(tests, func() Test {
		t := newOkTest("rank 1", []headerValue{
			{"rank", "1"},
		})
		t.want = "NOTICE: #RANK is 1(HARD)"
		return *t
	}())

	tests = append(tests, func() Test {
		t := newOkTest("total under 100", []headerValue{
			{"total", "99.9999999999999"},
		})
		t.want = "WARNING: #TOTAL is under 100: 99.9999999999999"
		return *t
	}())

	tests = append(tests, func() Test {
		t := newOkTest("high total", []headerValue{
			{"total", "1000"},
		})
		t.bmsFile.TotalNotes = 2000
		t.want = "NOTICE: #TOTAL is very high(TotalNotes=2000): 1000"
		return *t
	}())
	tests = append(tests, func() Test {
		t := newOkTest("low total", []headerValue{
			{"total", "200"},
		})
		t.bmsFile.TotalNotes = 2000
		t.want = "NOTICE: #TOTAL is very low(TotalNotes=2000): 200"
		return *t
	}())

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs := CheckHeaderCommands(test.bmsFile)
			if got := logs.String(); got != test.want {
				t.Errorf("got = %s\n\nwant = %s", got, test.want)
			}
		})
	}
}

func TestCheckTitleAndSubtitleHaveSameText(t *testing.T) {
	tests := []struct {
		name    string
		bmsFile *BmsFile
		want    *titleAndSubtitleHaveSameText
	}{
		{
			name: "detect",
			bmsFile: &BmsFile{Header: map[string]string{
				"title":    "songtitle another",
				"subtitle": "another",
			}},
			want: &titleAndSubtitleHaveSameText{subtitle: "another"},
		},
		{
			name: "detect2",
			bmsFile: &BmsFile{Header: map[string]string{
				"title":    "sHYPER",
				"subtitle": "hyper",
			}},
			want: &titleAndSubtitleHaveSameText{subtitle: "hyper"},
		},
		{
			name: "detect japanese",
			bmsFile: &BmsFile{Header: map[string]string{
				"title":    "曲名 アナザー",
				"subtitle": "アナザー",
			}},
			want: &titleAndSubtitleHaveSameText{subtitle: "アナザー"},
		},
		{
			name: "pass",
			bmsFile: &BmsFile{Header: map[string]string{
				"title":    "songtitle",
				"subtitle": "another",
			}},
			want: nil,
		},
		{
			name: "pass brackets",
			bmsFile: &BmsFile{Header: map[string]string{
				"title":    "songtitle [another]",
				"subtitle": "another",
			}},
			want: nil,
		},
		{
			name: "empty subtitle",
			bmsFile: &BmsFile{Header: map[string]string{
				"title":    "test",
				"subtitle": "",
			}},
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := CheckTitleAndSubtitleHaveSameText(test.bmsFile)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("got = %v, want = %v", got, test.want)
			}
		})
	}
}

func TestCheckIndexedDefinitionsHaveInvalidValue(t *testing.T) {
	strSmallestNonzeroFloat := strconv.FormatFloat(math.SmallestNonzeroFloat64, 'f', -1, 64)
	strMaxFloat := strconv.FormatFloat(math.MaxFloat64, 'f', -1, 64)

	type Test struct {
		name    string
		bmsFile *BmsFile
		wantMds []missingIndexedDefinition
		wantEds []emptyDefinition
		wantIvs []invalidValueOfIndexedCommand
		wantNwd *noWavExtDefs
	}

	tests := []Test{
		{
			name:    "missing",
			bmsFile: &BmsFile{},
			wantMds: func() []missingIndexedDefinition {
				mds := []missingIndexedDefinition{}
				for _, command := range INDEXED_COMMANDS {
					if command.Necessity != Unnecessary {
						mds = append(mds, missingIndexedDefinition{command: command})
					}
				}
				return mds
			}(),
		},
		{
			name: "empty",
			bmsFile: &BmsFile{
				HeaderWav:         []indexedDefinition{{CommandName: "wav", Index: "01", Value: ""}},
				HeaderBmp:         []indexedDefinition{{CommandName: "bmp", Index: "01", Value: ""}},
				HeaderExtendedBpm: []indexedDefinition{{CommandName: "bpm", Index: "01", Value: ""}},
				HeaderStop:        []indexedDefinition{{CommandName: "stop", Index: "01", Value: ""}},
				HeaderScroll:      []indexedDefinition{{CommandName: "scroll", Index: "01", Value: ""}},
			},
			wantEds: func() []emptyDefinition {
				eds := []emptyDefinition{
					{definition: indexedDefinition{CommandName: "wav", Index: "01", Value: ""}},
					{definition: indexedDefinition{CommandName: "bmp", Index: "01", Value: ""}},
					{definition: indexedDefinition{CommandName: "bpm", Index: "01", Value: ""}},
					{definition: indexedDefinition{CommandName: "stop", Index: "01", Value: ""}},
					{definition: indexedDefinition{CommandName: "scroll", Index: "01", Value: ""}},
				}
				return eds
			}(),
		},
		{
			name: "ok",
			bmsFile: &BmsFile{
				HeaderWav: []indexedDefinition{
					{CommandName: "wav", Index: "01", Value: "test.wav"},
					{CommandName: "wav", Index: "01", Value: ".Wav"},
					//{CommandName: "wav", Index: "02", Value: "test.OGG"},
					//{CommandName: "wav", Index: "03", Value: "test.flac"},
				},
				HeaderBmp: []indexedDefinition{
					{CommandName: "bmp", Index: "01", Value: "test.BMP"},
					{CommandName: "bmp", Index: "02", Value: "テスト.mP4"},
				},
				HeaderExtendedBpm: []indexedDefinition{
					{CommandName: "bpm", Index: "01", Value: "100"},
					{CommandName: "bpm", Index: "02", Value: "0.1"},
					{CommandName: "bpm", Index: "03", Value: "9999.9999"},
					{CommandName: "bpm", Index: "04", Value: strSmallestNonzeroFloat},
					{CommandName: "bpm", Index: "05", Value: strMaxFloat},
				},
				HeaderStop: []indexedDefinition{
					{CommandName: "stop", Index: "01", Value: "10"},
					{CommandName: "stop", Index: "02", Value: strSmallestNonzeroFloat},
					{CommandName: "stop", Index: "03", Value: strMaxFloat},
				},
				HeaderScroll: []indexedDefinition{
					{CommandName: "scroll", Index: "01", Value: "1.5"},
					{CommandName: "scroll", Index: "02", Value: "0"},
					{CommandName: "scroll", Index: "03", Value: "-100"},
					{CommandName: "scroll", Index: "04", Value: strMaxFloat},
					{CommandName: "scroll", Index: "05", Value: "-" + strMaxFloat},
				},
			},
		},
		{
			name: "no wav ext",
			bmsFile: &BmsFile{
				HeaderWav: []indexedDefinition{
					{CommandName: "wav", Index: "01", Value: "test.wav"},
					{CommandName: "wav", Index: "02", Value: "test.ogg"},
					{CommandName: "wav", Index: "03", Value: "test.mp3"},
					{CommandName: "wav", Index: "04", Value: "test.flac"},
				},
			},
			wantNwd: &noWavExtDefs{noWavExtDefs: []indexedDefinition{
				{CommandName: "wav", Index: "02", Value: "test.ogg"},
				{CommandName: "wav", Index: "03", Value: "test.mp3"},
				{CommandName: "wav", Index: "04", Value: "test.flac"},
			}},
		},
	}

	invalidBmsFile := &BmsFile{
		HeaderWav: []indexedDefinition{
			{CommandName: "wav", Index: "01", Value: "test"},
			{CommandName: "wav", Index: "02", Value: "test.wav_"},
			{CommandName: "wav", Index: "03", Value: "12345.wave"},
			{CommandName: "wav", Index: "04", Value: "12345.aif"},
			{CommandName: "wav", Index: "05", Value: "test.mpg"},
		},
		HeaderBmp: []indexedDefinition{
			{CommandName: "bmp", Index: "01", Value: "テスト"},
		},
		HeaderExtendedBpm: []indexedDefinition{
			{CommandName: "bpm", Index: "01", Value: "0"},
			{CommandName: "bpm", Index: "02", Value: "-1.1"},
			{CommandName: "bpm", Index: "03", Value: "aiueo"},
		},
		HeaderStop: []indexedDefinition{
			{CommandName: "stop", Index: "01", Value: "0.0"},
			{CommandName: "stop", Index: "02", Value: "-5555"},
			{CommandName: "stop", Index: "03", Value: "ストップ"},
		},
		HeaderScroll: []indexedDefinition{
			{CommandName: "scroll", Index: "01", Value: "スクロール"},
			{CommandName: "scroll", Index: "02", Value: "1/2"},
		},
	}
	tests = append(tests, Test{
		name:    "invalid",
		bmsFile: invalidBmsFile,
		wantIvs: func() (ivs []invalidValueOfIndexedCommand) {
			ids := []indexedDefinition{}
			ids = append(ids, invalidBmsFile.HeaderWav...)
			ids = append(ids, invalidBmsFile.HeaderBmp...)
			ids = append(ids, invalidBmsFile.HeaderExtendedBpm...)
			ids = append(ids, invalidBmsFile.HeaderStop...)
			ids = append(ids, invalidBmsFile.HeaderScroll...)
			for _, id := range ids {
				ivs = append(ivs, invalidValueOfIndexedCommand{definition: id})
			}
			return ivs
		}(),
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotMds, gotEds, gotIvs, gotNwd := CheckIndexedDefinitionsHaveInvalidValue(test.bmsFile)
			if !reflect.DeepEqual(gotMds, test.wantMds) {
				t.Errorf("mds: got = %v\nwant = %v", gotMds, test.wantMds)
			}
			if !reflect.DeepEqual(gotEds, test.wantEds) {
				t.Errorf("eds: got = %v\nwant = %v", gotEds, test.wantEds)
			}
			if !reflect.DeepEqual(gotIvs, test.wantIvs) {
				t.Errorf("ivs: got = %v\nwant = %v", gotIvs, test.wantIvs)
			}
			if !reflect.DeepEqual(gotNwd, test.wantNwd) {
				t.Errorf("hnd: got = %v\nwant = %v", gotNwd, test.wantNwd)
			}
		})
	}
}
