package bmson

import "encoding/json"

type Bmson struct {
	Version        string         `json:"version" validate:"required"`
	Info           *BmsonInfo     `json:"info" validate:"required"`
	Lines          []BarLine      `json:"lines"`
	Bpm_events     []BpmEvent     `json:"bpm_events"`
	Stop_events    []StopEvent    `json:"stop_events"`
	Sound_channels []SoundChannel `json:"sound_channels" validate:"required"`
	Bga            *BGA           `json:"bga"`
	Scroll_events  []ScrollEvent  `json:"scroll_events"` // beatoraja expansion
}

type BmsonInfo struct {
	Title          string   `json:"title"`
	Subtitle       string   `json:"subtitle"`
	Artist         string   `json:"artist"`
	Subartists     []string `json:"subartists"`
	Genre          string   `json:"genre"`
	Mode_hint      string   `json:"mode_hint"`
	Chart_name     string   `json:"chart_name"`
	Level          int      `json:"level"`
	Init_bpm       float64  `json:"init_bpm" validate:"required"`
	Judge_rank     float64  `json:"judge_rank"`
	Total          float64  `json:"total"`
	Back_image     string   `json:"back_image"`
	Eyecatch_image string   `json:"eyecatch_image"`
	Title_image    string   `json:"title_image"`
	Banner_image   string   `json:"banner_image"`
	Preview_music  string   `json:"preview_music"`
	Resolution     int      `json:"resolution"`
	Ln_type        int      `json:"ln_type"` // beatoraja expansion
}

type BarLine struct {
	Y int `json:"y" validate:"required"`
}

type SoundChannel struct {
	Name  string `json:"name" validate:"required"`
	Notes []Note `json:"notes" validate:"required"`
}

type Note struct {
	X  interface{} `json:"x"`
	Y  int         `json:"y" validate:"required"`
	L  int         `json:"l"`
	C  bool        `json:"c" validate:"required"`
	T  int         `json:"t"`  // beatoraja expansion
	Up bool        `json:"up"` // beatoraja expansion
}

type BpmEvent struct {
	Y   int     `json:"y" validate:"required"`
	Bpm float64 `json:"bpm" validate:"required"`
}

type StopEvent struct {
	Y        int `json:"y" validate:"required"`
	Duration int `json:"duration" validate:"required"`
}

type BGA struct {
	Bga_header   []BGAHeader `json:"bga_header" validate:"required"`
	Bga_events   []BGAEvent  `json:"bga_events"`
	Layer_events []BGAEvent  `json:"layer_events"`
	Poor_events  []BGAEvent  `json:"poor_events"`
}

type BGAHeader struct {
	Id   int    `json:"id" validate:"required"`
	Name string `json:"name" validate:"required"`
}

type BGAEvent struct {
	Y  int `json:"y" validate:"required"`
	Id int `json:"id" validate:"required"`
}

type ScrollEvent struct { // beatoraja expansion
	Y    int     `json:"y" validate:"required"`
	Rate float64 `json:"rate" validate:"required"`
}
}

func LoadBmson(bytes []byte) (bmson *Bmson, _ error) {
	if err := json.Unmarshal(bytes, &bmson); err != nil {
		return nil, err
	}
	return bmson, nil
}
