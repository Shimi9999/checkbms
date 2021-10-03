package bmson

import "encoding/json"

type Bmson struct {
	Version        string         `json:"version" necessity:"necessary"`
	Info           *BmsonInfo     `json:"info" necessity:"necessary"`
	Lines          []BarLine      `json:"lines"`
	Bpm_events     []BpmEvent     `json:"bpm_events"`
	Stop_events    []StopEvent    `json:"stop_events"`
	Sound_channels []SoundChannel `json:"sound_channels" necessity:"necessary"`
	Bga            *BGA           `json:"bga"`
	Scroll_events  []ScrollEvent  `json:"scroll_events"` // beatoraja expansion
}

type BmsonInfo struct {
	Title          string   `json:"title" necessity:"semi-necessary"`
	Subtitle       string   `json:"subtitle"`
	Artist         string   `json:"artist" necessity:"semi-necessary"`
	Subartists     []string `json:"subartists"`
	Genre          string   `json:"genre" necessity:"semi-necessary"`
	Mode_hint      string   `json:"mode_hint" necessity:"semi-necessary"`
	Chart_name     string   `json:"chart_name"`
	Level          int      `json:"level" necessity:"semi-necessary"`
	Init_bpm       float64  `json:"init_bpm" necessity:"necessary"`
	Judge_rank     float64  `json:"judge_rank" necessity:"semi-necessary"`
	Total          float64  `json:"total" necessity:"semi-necessary"`
	Back_image     string   `json:"back_image"`
	Eyecatch_image string   `json:"eyecatch_image"`
	Title_image    string   `json:"title_image"`
	Banner_image   string   `json:"banner_image"`
	Preview_music  string   `json:"preview_music"`
	Resolution     int      `json:"resolution"`
	Ln_type        int      `json:"ln_type"` // beatoraja expansion
}

type BarLine struct {
	Y int `json:"y" necessity:"necessary"`
}

type SoundChannel struct {
	Name  string `json:"name" necessity:"necessary"`
	Notes []Note `json:"notes" necessity:"necessary"`
}

type Note struct {
	X  interface{} `json:"x"`
	Y  int         `json:"y" necessity:"necessary"`
	L  int         `json:"l"`
	C  bool        `json:"c" necessity:"necessary"`
	T  int         `json:"t"`  // beatoraja expansion
	Up bool        `json:"up"` // beatoraja expansion
}

type BpmEvent struct {
	Y   int     `json:"y" necessity:"necessary"`
	Bpm float64 `json:"bpm" necessity:"necessary"`
}

type StopEvent struct {
	Y        int `json:"y" necessity:"necessary"`
	Duration int `json:"duration" necessity:"necessary"`
}

type BGA struct {
	Bga_header   []BGAHeader `json:"bga_header"`
	Bga_events   []BGAEvent  `json:"bga_events"`
	Layer_events []BGAEvent  `json:"layer_events"`
	Poor_events  []BGAEvent  `json:"poor_events"`
}

type BGAHeader struct {
	Id   int    `json:"id" necessity:"necessary"`
	Name string `json:"name" necessity:"necessary"`
}

type BGAEvent struct {
	Y  int `json:"y" necessity:"necessary"`
	Id int `json:"id" necessity:"necessary"`
}

type ScrollEvent struct { // beatoraja expansion
	Y    int     `json:"y" necessity:"necessary"`
	Rate float64 `json:"rate" necessity:"necessary"`
}

func LoadBmson(bytes []byte) (bmson *Bmson, _ error) {
	if err := json.Unmarshal(bytes, &bmson); err != nil {
		return nil, err
	}
	return bmson, nil
}
