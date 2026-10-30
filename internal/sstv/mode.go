package sstv

import (
	"fmt"
	"strings"
)

// ModeFamily selects a protocol timing/layout implementation.
type ModeFamily string

type HeaderKind string

const (
	HeaderVIS       HeaderKind = "vis7"
	HeaderMMSSTV    HeaderKind = "mmsstv16"
	HeaderNarrowVIS HeaderKind = "nvis"
	HeaderAVT       HeaderKind = "avt90"
)

const (
	FamilyRobot   ModeFamily = "robot"
	FamilyMartin  ModeFamily = "martin"
	FamilyScottie ModeFamily = "scottie"
	FamilyPD      ModeFamily = "pd"
	FamilyWraase  ModeFamily = "wraase"
	FamilyPasokon ModeFamily = "pasokon"
	FamilyMP      ModeFamily = "mmsstv_mp"
	FamilyMRML    ModeFamily = "mmsstv_mrml"
	FamilyMCN     ModeFamily = "mmsstv_mcn"
	FamilyAVT     ModeFamily = "avt"
)

// ModeSpec describes one SSTV mode. Timing fields are milliseconds. CodecReady
// is true only when both a matching encoder and decoder are implemented.
type ModeSpec struct {
	ID          string
	Name        string
	Family      ModeFamily
	VIS         uint8
	Header      HeaderKind
	CodecReady  bool
	ExtendedVIS uint16
	NarrowVIS   uint8
	Width       int
	Height      int
	BodyTimeMS  int
	Description string

	// RGB timing. RGBScanMS is used when channel times differ, notably SC2.
	SyncMS       float64
	PorchMS      float64
	ScanMS       float64
	RGBScanMS    [3]float64
	GapMS        float64
	SyncAtStart  bool
	ChannelOrder string

	// Robot YCbCr timing.
	SyncPorchMS     float64
	LumaMS          float64
	ChromaMS        float64
	SeparatorMS     float64
	ColorPorchMS    float64
	RobotSequential bool
	Monochrome      bool

	// PD, MP and MR/ML channel timing.
	ChannelScanMS float64
	PixelHoldMS   float64

	PorchEachChannel bool
	NarrowBand       bool
}

const (
	Robot36ID        = "robot36"
	DefaultRate      = 48_000
	DefaultLevel     = 0.80
	VideoMinHz       = 1_500.0
	VideoMaxHz       = 2_300.0
	VideoCenterHz    = 1_900.0
	HorizontalSync   = 1_200.0
	NarrowVideoMinHz = 2_044.0
	NarrowVideoMaxHz = 2_300.0
)

func mode(id, name string, family ModeFamily, vis uint8, width, height, bodyMS int) ModeSpec {
	return ModeSpec{ID: id, Name: name, Family: family, VIS: vis, Header: HeaderVIS,
		CodecReady: true, Width: width, Height: height, BodyTimeMS: bodyMS,
		Description: string(family) + " family timing profile"}
}

var modeOrder = []string{
	"bw8", "bw12", "robot24", "robot36", "robot72", "avt90",
	"scottie_s1", "scottie_s2", "scottie_dx", "martin_m1", "martin_m2",
	"sc2_180", "sc2_120", "sc2_60",
	"pd50", "pd90", "pd120", "pd160", "pd180", "pd240", "pd290",
	"pasokon_p3", "pasokon_p5", "pasokon_p7",
	"mp73", "mp115", "mp140", "mp175",
	"mr73", "mr90", "mr115", "mr140", "mr175",
	"ml180", "ml240", "ml280", "ml320",
	"mp73_n", "mp110_n", "mp140_n", "mc110_n", "mc140_n", "mc180_n",
}

var modeTable = buildModeTable()

func buildModeTable() map[string]ModeSpec {
	m := make(map[string]ModeSpec, len(modeOrder))
	add := func(s ModeSpec) { m[s.ID] = s }

	// Robot monochrome and colour modes, using MMSSTV-compatible rasters.
	bw8 := mode("bw8", "B/W 8", FamilyRobot, 2, 160, 120, 8_028)
	bw8.SyncMS, bw8.ScanMS, bw8.Monochrome = 7, 59.9, true
	add(bw8)
	bw12 := mode("bw12", "B/W 12", FamilyRobot, 6, 160, 120, 12_000)
	bw12.SyncMS, bw12.ScanMS, bw12.Monochrome = 7, 93, true
	add(bw12)

	r24 := mode("robot24", "Robot 24", FamilyRobot, 4, 320, 120, 24_000)
	r24.SyncMS, r24.LumaMS, r24.ChromaMS = 12, 88, 44
	r24.SeparatorMS, r24.RobotSequential = 6, true
	add(r24)

	r36 := mode(Robot36ID, "Robot 36", FamilyRobot, 8, 320, 240, 36_000)
	r36.SyncMS, r36.SyncPorchMS = 9, 3
	r36.LumaMS, r36.ChromaMS = 88, 44
	r36.SeparatorMS, r36.ColorPorchMS = 4.5, 1.5
	add(r36)

	r72 := mode("robot72", "Robot 72", FamilyRobot, 12, 320, 240, 72_000)
	r72.SyncMS, r72.SyncPorchMS = 9, 3
	r72.LumaMS, r72.ChromaMS = 138, 69
	r72.SeparatorMS, r72.ColorPorchMS, r72.RobotSequential = 4.5, 1.5, true
	add(r72)

	// AVT 90 is synchronised by its three VIS headers and digital countdown,
	// therefore it has no horizontal line-sync pulses.
	avt := mode("avt90", "AVT 90", FamilyAVT, 68, 256, 240, 90_000)
	avt.Header, avt.ScanMS, avt.ChannelOrder = HeaderAVT, 125, "RGB"
	add(avt)

	// Standard RGB modes.
	for _, v := range []struct {
		id, name string
		vis      int
		scan     float64
		body     int
	}{
		{"scottie_s1", "Scottie 1", 60, 138.24, 109_624},
		{"scottie_s2", "Scottie 2", 56, 88.064, 71_089},
		{"scottie_dx", "ScottieDX", 76, 345.6, 268_894},
	} {
		s := mode(v.id, v.name, FamilyScottie, uint8(v.vis), 320, 256, v.body)
		s.SyncMS, s.PorchMS, s.ScanMS = 9, 1.5, v.scan
		s.SyncAtStart, s.ChannelOrder = false, "GBR"
		add(s)
	}
	for _, v := range []struct {
		id, name string
		vis      int
		scan     float64
		body     int
	}{
		{"martin_m1", "Martin 1", 44, 146.432, 114_290},
		{"martin_m2", "Martin 2", 40, 73.216, 58_064},
	} {
		s := mode(v.id, v.name, FamilyMartin, uint8(v.vis), 320, 256, v.body)
		s.SyncMS, s.PorchMS, s.ScanMS = 4.862, 0.572, v.scan
		s.SyncAtStart, s.ChannelOrder = true, "GBR"
		add(s)
	}

	// Wraase SC-2 uses R:G:B = 2:4:2 for SC2-60/120. SC2-180 uses
	// equal component times. The per-channel values preserve those ratios.
	for _, v := range []struct {
		id, name string
		vis      int
		scans    [3]float64
		body     int
	}{
		{"sc2_180", "SC2 180", 55, [3]float64{235, 235, 235}, 182_000},
		{"sc2_120", "SC2 120", 63, [3]float64{117, 235, 117}, 122_000},
		{"sc2_60", "SC2 60", 59, [3]float64{58, 117, 58}, 62_000},
	} {
		s := mode(v.id, v.name, FamilyWraase, uint8(v.vis), 320, 256, v.body)
		s.SyncMS, s.RGBScanMS, s.ChannelOrder = 5, v.scans, "RGB"
		s.SyncAtStart = true
		add(s)
	}

	// PD family: each super-line transmits two luminance rows and shared chroma.
	for _, v := range []struct {
		id, name  string
		vis, w, h int
		scan      float64
		body      int
	}{
		{"pd50", "PD50", 93, 320, 256, 91.52, 49_688},
		{"pd90", "PD90", 99, 320, 256, 170.24, 89_995},
		{"pd120", "PD120", 95, 640, 496, 121.6, 126_112},
		{"pd160", "PD160", 98, 512, 400, 195.584, 160_894},
		{"pd180", "PD180", 96, 640, 496, 183.04, 187_065},
		{"pd240", "PD240", 97, 640, 496, 244.48, 248_017},
		{"pd290", "PD290", 94, 800, 616, 228.8, 288_702},
	} {
		s := mode(v.id, v.name, FamilyPD, uint8(v.vis), v.w, v.h, v.body)
		s.SyncMS, s.PorchMS, s.ChannelScanMS = 20, 2.08, v.scan
		s.SyncAtStart, s.ChannelOrder = true, "YCrCbY"
		add(s)
	}

	// Pasokon P3/P5/P7. All three use 1965 timing units per scan line.
	for _, v := range []struct {
		id, name        string
		vis             int
		sync, scan, gap float64
		body            int
	}{
		{"pasokon_p3", "P3", 113, 25.0 / 4800 * 1000, 640.0 / 4800 * 1000, 5.0 / 4800 * 1000, 203_050},
		{"pasokon_p5", "P5", 114, 25.0 / 3200 * 1000, 640.0 / 3200 * 1000, 5.0 / 3200 * 1000, 304_575},
		{"pasokon_p7", "P7", 115, 25.0 / 2400 * 1000, 640.0 / 2400 * 1000, 5.0 / 2400 * 1000, 406_100},
	} {
		s := mode(v.id, v.name, FamilyPasokon, uint8(v.vis), 640, 496, v.body)
		s.SyncMS, s.PorchMS, s.ScanMS, s.GapMS = v.sync, v.gap, v.scan, v.gap
		s.SyncAtStart, s.ChannelOrder = true, "RGB"
		add(s)
	}

	// MMSSTV 16-bit VIS MP modes. Each of 128 super-lines carries Y(odd),
	// Cr, Cb and Y(even), with a shared chroma pair.
	for _, v := range []struct {
		id, name string
		code     uint16
		tw       float64
		seconds  int
	}{
		{"mp73", "MP73", 0x2523, 140, 73},
		{"mp115", "MP115", 0x2923, 223, 115},
		{"mp140", "MP140", 0x2a23, 270, 140},
		{"mp175", "MP175", 0x2c23, 340, 175},
	} {
		s := mode(v.id, v.name, FamilyMP, 0x23, 320, 256, v.seconds*1000)
		s.Header, s.ExtendedVIS, s.SyncMS, s.PorchMS, s.ChannelScanMS = HeaderMMSSTV, v.code, 9, 1, v.tw
		s.ChannelOrder = "YCrCbY"
		add(s)
	}

	// MR/ML modes transmit full-width luminance and horizontally compressed
	// chroma on every row. Extended VIS identifies the exact timing profile.
	for _, v := range []struct {
		id, name string
		code     uint16
		w, h     int
		tw       float64
		seconds  int
	}{
		{"mr73", "MR73", 0x4523, 320, 256, 138, 73},
		{"mr90", "MR90", 0x4623, 320, 256, 171, 90},
		{"mr115", "MR115", 0x4923, 320, 256, 220, 115},
		{"mr140", "MR140", 0x4a23, 320, 256, 269, 140},
		{"mr175", "MR175", 0x4c23, 320, 256, 337, 175},
		{"ml180", "ML180", 0x8523, 640, 496, 176.5, 180},
		{"ml240", "ML240", 0x8623, 640, 496, 236.5, 240},
		{"ml280", "ML280", 0x8923, 640, 496, 277.5, 280},
		{"ml320", "ML320", 0x8a23, 640, 496, 317.5, 320},
	} {
		s := mode(v.id, v.name, FamilyMRML, 0x23, v.w, v.h, v.seconds*1000)
		s.Header, s.ExtendedVIS = HeaderMMSSTV, v.code
		s.SyncMS, s.PorchMS, s.ChannelScanMS, s.PixelHoldMS = 9, 1, v.tw, 0.1
		s.ChannelOrder = "YCrCb"
		add(s)
	}

	// Narrow-band modes use the MMSSTV 24-bit FSK N-VIS header and 2044..2300 Hz
	// picture range, never the conventional 1500..2300 Hz video scale.
	for _, v := range []struct {
		id, name string
		family   ModeFamily
		code     uint8
		tw       float64
		rgb      [3]float64
		body     int
	}{
		{"mp73_n", "MP73-N", FamilyMP, 0x02, 140, [3]float64{}, 73_000},
		{"mp110_n", "MP110-N", FamilyMP, 0x04, 212, [3]float64{}, 110_000},
		{"mp140_n", "MP140-N", FamilyMP, 0x05, 270, [3]float64{}, 140_000},
		{"mc110_n", "MC110-N", FamilyMCN, 0x14, 0, [3]float64{143, 143, 143}, 110_000},
		{"mc140_n", "MC140-N", FamilyMCN, 0x15, 0, [3]float64{180, 180, 180}, 140_000},
		{"mc180_n", "MC180-N", FamilyMCN, 0x16, 0, [3]float64{232, 232, 232}, 180_000},
	} {
		s := mode(v.id, v.name, v.family, 0, 320, 256, v.body)
		s.Header, s.NarrowVIS, s.NarrowBand = HeaderNarrowVIS, v.code, true
		if v.family == FamilyMP {
			s.ChannelScanMS, s.SyncMS, s.PorchMS = v.tw, 9, 1
			s.ChannelOrder = "YCrCbY"
		} else {
			s.SyncMS, s.PorchMS, s.RGBScanMS, s.ChannelOrder = 8, 0.5, v.rgb, "RGB"
		}
		add(s)
	}
	return m
}

// LookupMode accepts a canonical ID, display name or common alias.
func LookupMode(id string) (ModeSpec, error) {
	key := normalizeModeID(id)
	aliases := map[string]string{
		"b/w 8": "bw8", "bw 8": "bw8", "robot bw 8": "bw8",
		"b/w 12": "bw12", "bw 12": "bw12", "robot bw 12": "bw12",
		"robot 24": "robot24", "r24": "robot24", "robot 36": "robot36", "r36": "robot36", "robot 72": "robot72", "r72": "robot72",
		"avt 90": "avt90", "scottie 1": "scottie_s1", "scottie s1": "scottie_s1",
		"scottie 2": "scottie_s2", "scottie s2": "scottie_s2", "scottie dx": "scottie_dx", "scottiedx": "scottie_dx",
		"martin 1": "martin_m1", "m1": "martin_m1", "martin 2": "martin_m2", "m2": "martin_m2",
		"sc2 180": "sc2_180", "sc2-180": "sc2_180", "sc2 120": "sc2_120", "sc2-120": "sc2_120", "sc2 60": "sc2_60", "sc2-60": "sc2_60",
		"pd 50": "pd50", "pd 90": "pd90", "pd 120": "pd120", "pd 160": "pd160", "pd 180": "pd180", "pd 240": "pd240", "pd 290": "pd290",
		"p3": "pasokon_p3", "pasokon p3": "pasokon_p3", "p5": "pasokon_p5", "pasokon p5": "pasokon_p5", "p7": "pasokon_p7", "pasokon p7": "pasokon_p7",
		"mp73 n": "mp73_n", "mp110 n": "mp110_n", "mp140 n": "mp140_n", "mc110 n": "mc110_n", "mc140 n": "mc140_n", "mc180 n": "mc180_n",
	}
	if alias, ok := aliases[key]; ok {
		key = alias
	}
	if spec, ok := modeTable[key]; ok {
		return spec, nil
	}
	for _, spec := range modeTable {
		if normalizeModeID(spec.Name) == key {
			return spec, nil
		}
	}
	return ModeSpec{}, fmt.Errorf("unsupported SSTV mode %q", id)
}

func normalizeModeID(id string) string {
	// Accept the full-width and typographic hyphens often used in copied
	// mode lists (for example MP73－N) as the canonical ASCII hyphen.
	id = strings.NewReplacer("－", "-", "‐", "-", "‑", "-", "–", "-", "—", "-").Replace(id)
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(id)), " "))
}

// ModeFromVIS returns modes with an ordinary 7-bit VIS header.
func ModeFromVIS(code uint8) (ModeSpec, bool) {
	for _, id := range modeOrder {
		spec := modeTable[id]
		if (spec.Header == HeaderVIS || spec.Header == HeaderAVT) && spec.VIS == code {
			return spec, true
		}
	}
	return ModeSpec{}, false
}

// ModeFromExtendedVIS resolves the full 16-bit MMSSTV mode identifier.
func ModeFromExtendedVIS(code uint16) (ModeSpec, bool) {
	for _, id := range modeOrder {
		spec := modeTable[id]
		if spec.Header == HeaderMMSSTV && spec.ExtendedVIS == code {
			return spec, true
		}
	}
	return ModeSpec{}, false
}

// ModeFromNarrowVIS resolves a 6-bit N-VIS code.
func ModeFromNarrowVIS(code uint8) (ModeSpec, bool) {
	for _, id := range modeOrder {
		spec := modeTable[id]
		if spec.Header == HeaderNarrowVIS && spec.NarrowVIS == code {
			return spec, true
		}
	}
	return ModeSpec{}, false
}

// Modes returns the exact operator-requested 43-mode order.
func Modes() []ModeSpec {
	out := make([]ModeSpec, 0, len(modeOrder))
	for _, id := range modeOrder {
		out = append(out, modeTable[id])
	}
	return out
}

// BodyDurationMS returns the protocol image-body duration, excluding headers.
func BodyDurationMS(spec ModeSpec) float64 {
	switch spec.Family {
	case FamilyRobot:
		if spec.Monochrome {
			return float64(spec.Height) * (spec.SyncMS + spec.ScanMS)
		}
		lineMS := spec.SyncMS + spec.SyncPorchMS + spec.LumaMS + spec.SeparatorMS + spec.ColorPorchMS + spec.ChromaMS
		if spec.RobotSequential {
			lineMS += spec.SeparatorMS + spec.ColorPorchMS + spec.ChromaMS
		}
		return float64(spec.Height) * lineMS
	case FamilyMartin:
		return float64(spec.Height) * (spec.SyncMS + 4*spec.PorchMS + 3*spec.ScanMS)
	case FamilyScottie:
		return float64(spec.Height) * (spec.SyncMS + 3*spec.PorchMS + 3*spec.ScanMS)
	case FamilyPD:
		return float64(spec.Height/2) * (spec.SyncMS + spec.PorchMS + 4*spec.ChannelScanMS)
	case FamilyWraase:
		return float64(spec.Height) * (spec.SyncMS + spec.RGBScanMS[0] + spec.RGBScanMS[1] + spec.RGBScanMS[2])
	case FamilyPasokon:
		return float64(spec.Height) * (spec.SyncMS + 4*spec.GapMS + 3*spec.ScanMS)
	case FamilyMP:
		return float64(spec.Height/2) * (spec.SyncMS + spec.PorchMS + 4*spec.ChannelScanMS)
	case FamilyMRML:
		return float64(spec.Height) * (spec.SyncMS + spec.PorchMS + 2*spec.ChannelScanMS + 3*spec.PixelHoldMS)
	case FamilyMCN:
		return float64(spec.Height) * (spec.SyncMS + spec.PorchMS + spec.RGBScanMS[0] + spec.RGBScanMS[1] + spec.RGBScanMS[2])
	case FamilyAVT:
		return float64(spec.Height) * 3 * spec.ScanMS
	default:
		return float64(spec.BodyTimeMS)
	}
}

func videoRange(spec ModeSpec) (float64, float64) {
	if spec.NarrowBand {
		return NarrowVideoMinHz, NarrowVideoMaxHz
	}
	return VideoMinHz, VideoMaxHz
}

func byteToVideoFrequencyForMode(value uint8, spec ModeSpec) float64 {
	lo, hi := videoRange(spec)
	return lo + float64(value)*(hi-lo)/255
}

func frequencyToByteForMode(frequency float64, spec ModeSpec) uint8 {
	lo, hi := videoRange(spec)
	v := (frequency - lo) * 255 / (hi - lo)
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v + 0.5)
}
