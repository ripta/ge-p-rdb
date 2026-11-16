package pfile

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Offsets holds the section pointers from the RDB header. Each value is a
// byte offset from the start of the file.
type Offsets struct {
	Data        int32
	PerPass     int32
	UnlockRaw   int32
	DataAcqTab  int32
	NexTab      int32
	NexAbortTab int32
	Tool        int32
	Exam        int32
	Series      int32
	Image       int32
	PS          int32
	GradData    int32
	CTTData     int32
}

// DAB is one data acquisition board's receiver range, inclusive.
type DAB struct {
	StartRcv int16
	StopRcv  int16
}

// RDBHeader is the decoded raw data header (rdb_hdr_rec).
type RDBHeader struct {
	Revision float32
	Offsets  Offsets

	// ScanDate is MM/DD/YYY, where YYY is years since 1900.
	ScanDate string
	// ScanTime is HH:MM.
	ScanTime string
	// Logo is "GE_MED_NMR" in files from GE scanners.
	Logo string

	FileContents    int16
	DataCollectType int16
	NPasses         int16
	NSlices         int16
	NEchoes         int16
	NAvs            int16
	// NFrames is the views per block, not counting the baseline view.
	NFrames   int16
	FrameSize uint16
	// PointSize is bytes per component: 2 (int16) or 4 (int32).
	PointSize int16
	DAXRes    uint16
	DAYRes    int16
	DAB       [4]DAB

	// User holds rhuser0..rhuser48.
	User [49]float32

	SpectralWidth float32
	CSIDims       int16
	XCSI          int16
	YCSI          int16
	ZCSI          int16
	ROILenX       float32
	ROILenY       float32
	ROILenZ       float32
	PSMPSFreq     uint32
	UserUsageTag  uint32

	// RawPassSize is the bytes of sample data per pass.
	RawPassSize int64
}

// Exam is the decoded subset of the exam header. The UID fields are packed
// binary, not text.
type Exam struct {
	// MagStrength is in gauss.
	MagStrength int32
	// DateTime is in Unix seconds.
	DateTime    int32
	ExamNumber  uint16
	PatientSex  int16
	RefPhy      string
	SysID       string
	HospName    string
	VersCre     string
	UniqSysID   string
	StudyUID    []byte
	PatientName string
	PatientID   string
	ReqNum      string
	DateOfBirth string
}

// Series is the decoded subset of the series header. The UID fields are
// packed binary, not text.
type Series struct {
	Position    int32
	Entry       int32
	SeriesNo    int32
	Description string
	AnRef       string
	SeriesUID   []byte
	LandmarkUID []byte
}

// Image is the decoded subset of the image header. Lengths are in mm.
// Times are in microseconds. Coordinates are R, A, S triples.
type Image struct {
	DFOV        float32
	ScanSpacing float32
	Loc         float32
	NEX         float32
	// User holds user0..user48.
	User [49]float32
	Ctr  [3]float32
	// TLHC, TRHC and BRHC are the top left, top right and bottom right
	// corners of the image plane.
	TLHC      [3]float32
	TRHC      [3]float32
	BRHC      [3]float32
	TR        int32
	TI        int32
	TE        int32
	RawRunNum int32
	NumEcho   int16
	// FlipAngle is in degrees.
	FlipAngle int16
	CTyp      int16
	FreqDir   int16
	PSDName   string
	ProjName  string
	PSDIName  string
	CoilName  string
	// ImageUID is packed binary, not text.
	ImageUID []byte
}

// AcqEntry is one entry of the data acquisition table. Entries are indexed
// by spatial slice.
//
// The GW points are R, A, S triples. They describe the slice plane in a
// frame centered on the image center (Image.Ctr). In the sample files,
// GWPoint1 and GWPoint3 share a row and GWPoint1 and GWPoint2 share a
// column. Their mapping to TLHC, TRHC and BRHC was not confirmed.
type AcqEntry struct {
	PassNumber int16
	// SliceInPass is the 1-based acquisition position within the pass.
	SliceInPass int16
	GWPoint1    [3]float32
	GWPoint2    [3]float32
	GWPoint3    [3]float32
}

// Header is everything decoded from the bytes before off_data.
type Header struct {
	RDB      RDBHeader
	Exam     Exam
	Series   Series
	Image    Image
	AcqTable []AcqEntry
}

// ParseScanTime parses ScanDate and ScanTime. The time has no zone, so it is
// returned in UTC.
func (h *RDBHeader) ParseScanTime() (time.Time, error) {
	parts := strings.Split(h.ScanDate, "/")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("pfile: bad scan date %q", h.ScanDate)
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return time.Time{}, fmt.Errorf("pfile: bad scan date %q", h.ScanDate)
		}
		nums[i] = n
	}
	var hh, mm int
	if _, err := fmt.Sscanf(h.ScanTime, "%d:%d", &hh, &mm); err != nil {
		return time.Time{}, fmt.Errorf("pfile: bad scan time %q", h.ScanTime)
	}
	return time.Date(1900+nums[2], time.Month(nums[0]), nums[1], hh, mm, 0, 0, time.UTC), nil
}

// NumReceivers returns the receiver count from DAB[0].
func (h *RDBHeader) NumReceivers() int {
	return int(h.DAB[0].StopRcv) - int(h.DAB[0].StartRcv) + 1
}

// sec is a bounds-checked little-endian view of one header section.
type sec struct {
	b []byte
}

func (s sec) i16(o int) int16   { return int16(binary.LittleEndian.Uint16(s.b[o:])) }
func (s sec) u16(o int) uint16  { return binary.LittleEndian.Uint16(s.b[o:]) }
func (s sec) i32(o int) int32   { return int32(binary.LittleEndian.Uint32(s.b[o:])) }
func (s sec) u32(o int) uint32  { return binary.LittleEndian.Uint32(s.b[o:]) }
func (s sec) i64(o int) int64   { return int64(binary.LittleEndian.Uint64(s.b[o:])) }
func (s sec) f32(o int) float32 { return math.Float32frombits(s.u32(o)) }

func (s sec) vec3(o int) [3]float32 {
	return [3]float32{s.f32(o), s.f32(o + 4), s.f32(o + 8)}
}

// str returns a NUL-terminated string from a fixed-size char field.
func (s sec) str(o, n int) string {
	b := s.b[o : o+n]
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimRight(string(b), " ")
}

func (s sec) raw(o, n int) []byte {
	return bytes.Clone(s.b[o : o+n])
}

// floats reads n consecutive float32 values into dst.
func (s sec) floats(dst []float32, o int) {
	for i := range dst {
		dst[i] = s.f32(o + 4*i)
	}
}

func parseRDB(s sec) RDBHeader {
	h := RDBHeader{
		Revision: s.f32(offRev),
		Offsets: Offsets{
			Data:        s.i32(offOffData),
			PerPass:     s.i32(offOffPerPass),
			UnlockRaw:   s.i32(offOffUnlockRaw),
			DataAcqTab:  s.i32(offOffDataAcqTab),
			NexTab:      s.i32(offOffNexTab),
			NexAbortTab: s.i32(offOffNexAbortTab),
			Tool:        s.i32(offOffTool),
			Exam:        s.i32(offOffExam),
			Series:      s.i32(offOffSeries),
			Image:       s.i32(offOffImage),
			PS:          s.i32(offOffPS),
			GradData:    s.i32(offOffGradData),
			CTTData:     s.i32(offOffCTTData),
		},
		ScanDate:        s.str(offScanDate, 10),
		ScanTime:        s.str(offScanTime, 8),
		Logo:            s.str(offLogo, 10),
		FileContents:    s.i16(offFileContents),
		DataCollectType: s.i16(offDataCollectType),
		NPasses:         s.i16(offNPasses),
		NSlices:         s.i16(offNSlices),
		NEchoes:         s.i16(offNEchoes),
		NAvs:            s.i16(offNAvs),
		NFrames:         s.i16(offNFrames),
		FrameSize:       s.u16(offFrameSize),
		PointSize:       s.i16(offPointSize),
		DAXRes:          s.u16(offDAXRes),
		DAYRes:          s.i16(offDAYRes),
		SpectralWidth:   s.f32(offSpectralWidth),
		CSIDims:         s.i16(offCSIDims),
		XCSI:            s.i16(offXCSI),
		YCSI:            s.i16(offYCSI),
		ZCSI:            s.i16(offZCSI),
		ROILenX:         s.f32(offROILenX),
		ROILenY:         s.f32(offROILenY),
		ROILenZ:         s.f32(offROILenZ),
		PSMPSFreq:       s.u32(offPSMPSFreq),
		UserUsageTag:    s.u32(offUserUsageTag),
		RawPassSize:     s.i64(offRawPassSize),
	}
	for i := range h.DAB {
		h.DAB[i] = DAB{StartRcv: s.i16(offDAB + 4*i), StopRcv: s.i16(offDAB + 4*i + 2)}
	}
	s.floats(h.User[:20], offRHUser0)
	s.floats(h.User[20:], offRHUser20)
	return h
}

func parseExam(s sec) Exam {
	return Exam{
		MagStrength: s.i32(exMagStrength),
		DateTime:    s.i32(exDateTime),
		ExamNumber:  s.u16(exNo),
		PatientSex:  s.i16(exPatSex),
		RefPhy:      s.str(exRefPhy, 65),
		SysID:       s.str(exSysID, 9),
		HospName:    s.str(exHospName, 33),
		VersCre:     s.str(exVersCre, 2),
		UniqSysID:   s.str(exUniqSysID, 16),
		StudyUID:    s.raw(exStudyUID, 32),
		PatientName: s.str(exPatName, 65),
		PatientID:   s.str(exPatID, 65),
		ReqNum:      s.str(exReqNum, 17),
		DateOfBirth: s.str(exDateOfBirth, 9),
	}
}

func parseSeries(s sec) Series {
	return Series{
		Position:    s.i32(sePosition),
		Entry:       s.i32(seEntry),
		SeriesNo:    s.i32(seNo),
		Description: s.str(seDesc, 65),
		AnRef:       s.str(seAnRef, 3),
		SeriesUID:   s.raw(seSeriesUID, 32),
		LandmarkUID: s.raw(seLandmarkUID, 32),
	}
}

func parseImage(s sec) Image {
	im := Image{
		DFOV:        s.f32(imDFOV),
		ScanSpacing: s.f32(imScanSpacing),
		Loc:         s.f32(imLoc),
		NEX:         s.f32(imNEX),
		Ctr:         s.vec3(imCtr),
		TLHC:        s.vec3(imTLHC),
		TRHC:        s.vec3(imTRHC),
		BRHC:        s.vec3(imBRHC),
		TR:          s.i32(imTR),
		TI:          s.i32(imTI),
		TE:          s.i32(imTE),
		RawRunNum:   s.i32(imRawRunNum),
		NumEcho:     s.i16(imNumEcho),
		FlipAngle:   s.i16(imFlip),
		CTyp:        s.i16(imCTyp),
		FreqDir:     s.i16(imFreqDir),
		PSDName:     s.str(imPSDName, 33),
		ProjName:    s.str(imProjName, 13),
		PSDIName:    s.str(imPSDIName, 13),
		CoilName:    s.str(imCName, 17),
		ImageUID:    s.raw(imImageUID, 32),
	}
	s.floats(im.User[:23], imUser0)
	s.floats(im.User[23:25], imUser23)
	s.floats(im.User[25:], imUser25)
	return im
}

func parseAcqTable(s sec, n int) []AcqEntry {
	t := make([]AcqEntry, n)
	for i := range t {
		o := i * acqEntrySize
		t[i] = AcqEntry{
			PassNumber:  s.i16(o + acqPassNumber),
			SliceInPass: s.i16(o + acqSliceInPass),
			GWPoint1:    s.vec3(o + acqGWPoint1),
			GWPoint2:    s.vec3(o + acqGWPoint2),
			GWPoint3:    s.vec3(o + acqGWPoint3),
		}
	}
	return t
}
