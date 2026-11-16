package pfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// samplesDir points at a directory of real P-files. The sample tests skip
// when it is empty.
var samplesDir = flag.String("samples", "", "directory holding P20480_GRE.7 and P21504_FSE.7")

// Section pointers copied from the 26.002 sample files.
const (
	synPerPass  = 4096
	synUnlock   = 20480
	synAcqTab   = 36864
	synNexTab   = 184320
	synNexAbort = 186372
	synTool     = 188424
	synPS       = 190472
	synExam     = 193660
	synSeries   = 195620
	synImage    = 198180
	synGrad     = 200628
	synCTT      = 208116
	synData     = 213684
)

type synth struct {
	b []byte
}

func (s *synth) i16(o int, v int16)   { binary.LittleEndian.PutUint16(s.b[o:], uint16(v)) }
func (s *synth) i32(o int, v int32)   { binary.LittleEndian.PutUint32(s.b[o:], uint32(v)) }
func (s *synth) i64(o int, v int64)   { binary.LittleEndian.PutUint64(s.b[o:], uint64(v)) }
func (s *synth) f32(o int, v float32) { binary.LittleEndian.PutUint32(s.b[o:], math.Float32bits(v)) }
func (s *synth) str(o int, v string)  { copy(s.b[o:], v) }

// synthShape is the data shape of a synthetic file.
type synthShape struct {
	PointSize int
	Receivers int
	Slices    int
	Echoes    int
	Views     int
	Points    int
}

// synthFile builds a single-pass 26.002 file. Each component of each sample
// encodes its own position, so reads can be checked exactly.
func synthFile(t *testing.T, sh synthShape) []byte {
	t.Helper()
	block := (sh.Views + 1) * sh.Points * 2 * sh.PointSize
	dataLen := sh.Receivers * sh.Slices * sh.Echoes * block
	s := &synth{b: make([]byte, synData+dataLen)}

	s.f32(offRev, 26.002)
	pointers := map[int]int32{
		offOffData: synData, offOffPerPass: synPerPass, offOffUnlockRaw: synUnlock,
		offOffDataAcqTab: synAcqTab, offOffNexTab: synNexTab, offOffNexAbortTab: synNexAbort,
		offOffTool: synTool, offOffExam: synExam, offOffSeries: synSeries, offOffImage: synImage,
		offOffPS: synPS, offOffGradData: synGrad, offOffCTTData: synCTT,
	}
	for o, v := range pointers {
		s.i32(o, v)
	}
	s.str(offScanDate, "04/11/118")
	s.str(offScanTime, "08:59")
	s.str(offLogo, "GE_MED_NMR")
	s.i16(offNPasses, 1)
	s.i16(offNSlices, int16(sh.Slices))
	s.i16(offNEchoes, int16(sh.Echoes))
	s.i16(offNFrames, int16(sh.Views))
	s.i16(offFrameSize, int16(sh.Points))
	s.i16(offPointSize, int16(sh.PointSize))
	s.i16(offDAB, 0)
	s.i16(offDAB+2, int16(sh.Receivers-1))
	s.f32(offRHUser0+4*3, 3.5)
	s.f32(offRHUser20+4*2, 22.5)
	s.i64(offRawPassSize, int64(dataLen))

	s.i32(synExam+exMagStrength, 30000)
	s.str(synExam+exHospName, "Somewhere")
	s.str(synSeries+seDesc, "Basic GRE scan")
	s.i32(synImage+imTR, 250000)
	s.str(synImage+imPSDName, "2dfast")
	s.f32(synImage+imUser25, 25.5)

	// Spatial slice i was acquired at position Slices-1-i.
	for i := range sh.Slices {
		o := synAcqTab + i*acqEntrySize
		s.i16(o+acqSliceInPass, int16(sh.Slices-i))
		s.f32(o+acqGWPoint1+4, float32(i)*10)
	}

	o := synData
	for r := range sh.Receivers {
		for sl := range sh.Slices {
			for e := range sh.Echoes {
				// The baseline view stays zero.
				o += sh.Points * 2 * sh.PointSize
				for v := range sh.Views {
					for p := range sh.Points {
						re, im := sampleValue(r, sl, e, v, p)
						if sh.PointSize == 2 {
							s.i16(o, int16(re))
							s.i16(o+2, int16(im))
						} else {
							s.i32(o, re)
							s.i32(o+4, im)
						}
						o += 2 * sh.PointSize
					}
				}
			}
		}
	}
	return s.b
}

func sampleValue(r, sl, e, v, p int) (int32, int32) {
	return int32(r*1000 + sl*100 + e*10 + p), -int32(v + 1)
}

type roundTripTest struct {
	Name  string
	Shape synthShape
}

var roundTripTests = []roundTripTest{
	{Name: "int16", Shape: synthShape{PointSize: 2, Receivers: 3, Slices: 4, Echoes: 2, Views: 5, Points: 6}},
	{Name: "int32", Shape: synthShape{PointSize: 4, Receivers: 3, Slices: 4, Echoes: 2, Views: 5, Points: 6}},
	{Name: "single block", Shape: synthShape{PointSize: 2, Receivers: 1, Slices: 1, Echoes: 1, Views: 1, Points: 1}},
}

func TestSyntheticRoundTrip(t *testing.T) {
	for _, test := range roundTripTests {
		t.Run(test.Name, func(t *testing.T) {
			sh := test.Shape
			b := synthFile(t, sh)
			f, err := New(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := f.DataErr(); err != nil {
				t.Fatalf("DataErr: %v", err)
			}
			want := Dims{Receivers: sh.Receivers, Slices: sh.Slices, Echoes: sh.Echoes, Views: sh.Views, Points: sh.Points}
			if got := f.Dims(); got != want {
				t.Fatalf("Dims = %+v, want %+v", got, want)
			}
			h := f.Header
			if h.Exam.MagStrength != 30000 || h.Series.Description != "Basic GRE scan" ||
				h.Image.TR != 250000 || h.Image.PSDName != "2dfast" || h.RDB.Logo != "GE_MED_NMR" {
				t.Errorf("header fields wrong: %+v / %+v / %+v", h.Exam, h.Series, h.Image)
			}
			if h.RDB.User[3] != 3.5 || h.RDB.User[22] != 22.5 || h.Image.User[25] != 25.5 {
				t.Errorf("user fields wrong: rh %v, image %v", h.RDB.User, h.Image.User)
			}
			st, err := h.RDB.ParseScanTime()
			if err != nil || !st.Equal(time.Date(2018, 4, 11, 8, 59, 0, 0, time.UTC)) {
				t.Errorf("ParseScanTime = %v, %v", st, err)
			}

			for r := range sh.Receivers {
				for sl := range sh.Slices {
					for e := range sh.Echoes {
						checkBlock(t, f, r, sl, e)
					}
				}
			}

			for sp := range sh.Slices {
				acq, err := f.AcqSlice(sp)
				if err != nil || acq != sh.Slices-1-sp {
					t.Errorf("AcqSlice(%d) = %d, %v; want %d", sp, acq, err, sh.Slices-1-sp)
				}
			}
			if _, err := f.ReadBlock(sh.Receivers, 0, 0); err == nil {
				t.Error("ReadBlock with receiver out of range: want error")
			}
		})
	}
}

func checkBlock(t *testing.T, f *File, r, sl, e int) {
	t.Helper()
	blk, err := f.ReadBlock(r, sl, e)
	if err != nil {
		t.Fatal(err)
	}
	for v := range blk.Views {
		for p := range blk.Points {
			re, im := sampleValue(r, sl, e, v, p)
			if got := blk.At(v, p); got != complex(float64(re), float64(im)) {
				t.Fatalf("block (%d,%d,%d) at (%d,%d) = %v, want %d%+di", r, sl, e, v, p, got, re, im)
			}
		}
	}
}

// eofReader returns io.EOF with a full read that reaches the end of the
// data. The io.ReaderAt contract allows that.
type eofReader struct {
	b []byte
}

func (e eofReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := bytes.NewReader(e.b).ReadAt(p, off)
	if err == nil && off+int64(n) == int64(len(e.b)) {
		err = io.EOF
	}
	return n, err
}

func TestEOFAtEndOfData(t *testing.T) {
	sh := synthShape{PointSize: 2, Receivers: 2, Slices: 1, Echoes: 1, Views: 2, Points: 2}
	b := synthFile(t, sh)
	f, err := New(eofReader{b: b}, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	checkBlock(t, f, sh.Receivers-1, 0, 0)
}

func TestUnsupportedRevision(t *testing.T) {
	b := synthFile(t, synthShape{PointSize: 2, Receivers: 1, Slices: 1, Echoes: 1, Views: 2, Points: 2})
	binary.LittleEndian.PutUint32(b, math.Float32bits(24.0))
	_, err := New(bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, ErrUnsupportedRevision) {
		t.Fatalf("err = %v, want ErrUnsupportedRevision", err)
	}
}

func TestTruncatedData(t *testing.T) {
	b := synthFile(t, synthShape{PointSize: 2, Receivers: 2, Slices: 2, Echoes: 1, Views: 4, Points: 4})
	b = b[:len(b)-1]
	f, err := New(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("New should still decode the header: %v", err)
	}
	if f.DataErr() == nil {
		t.Fatal("DataErr = nil for truncated file")
	}
	if _, err := f.ReadBlock(0, 0, 0); err == nil {
		t.Fatal("ReadBlock on truncated file: want error")
	}
}

type badHeaderTest struct {
	Name   string
	Mutate func(b []byte)
}

var badHeaderTests = []badHeaderTest{
	{
		Name:   "point size 3",
		Mutate: func(b []byte) { binary.LittleEndian.PutUint16(b[offPointSize:], 3) },
	},
	{
		Name:   "exam section overlaps series",
		Mutate: func(b []byte) { binary.LittleEndian.PutUint32(b[offOffExam:], synSeries-100) },
	},
	{
		Name:   "off_data past end of file",
		Mutate: func(b []byte) { binary.LittleEndian.PutUint32(b[offOffData:], uint32(len(b)+1)) },
	},
	{
		Name:   "empty receiver range",
		Mutate: func(b []byte) { binary.LittleEndian.PutUint16(b[offDAB+2:], 0xffff) },
	},
}

func TestBadHeader(t *testing.T) {
	for _, test := range badHeaderTests {
		t.Run(test.Name, func(t *testing.T) {
			b := synthFile(t, synthShape{PointSize: 2, Receivers: 1, Slices: 1, Echoes: 1, Views: 2, Points: 2})
			test.Mutate(b)
			if _, err := New(bytes.NewReader(b), int64(len(b))); err == nil {
				t.Error("New succeeded, want error")
			}
		})
	}
}

type sampleTest struct {
	Name      string
	Dims      Dims
	PointSize int16
	PSD       string
	Desc      string
	TR        int32
	RunNum    int32
}

// The expected values come from the two 26.002 files used to develop this
// package.
var sampleTests = []sampleTest{
	{Name: "P20480_GRE.7", Dims: Dims{8, 7, 1, 128, 128}, PointSize: 2, PSD: "2dfast", Desc: "Basic GRE scan", TR: 250000, RunNum: 20480},
	{Name: "P21504_FSE.7", Dims: Dims{8, 3, 1, 264, 256}, PointSize: 4, PSD: "fse-xl", Desc: "Sag T2 FRFSE 1mm^2x2mm", TR: 5000000, RunNum: 21504},
}

func TestSamples(t *testing.T) {
	if *samplesDir == "" {
		t.Skip("-samples not set")
	}
	for _, test := range sampleTests {
		t.Run(test.Name, func(t *testing.T) {
			path := filepath.Join(*samplesDir, test.Name)
			if _, err := os.Stat(path); err != nil {
				t.Skip(err)
			}
			f, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := f.DataErr(); err != nil {
				t.Fatal(err)
			}
			h := f.Header
			if got := f.Dims(); got != test.Dims {
				t.Errorf("Dims = %+v, want %+v", got, test.Dims)
			}
			if h.RDB.PointSize != test.PointSize || h.Image.PSDName != test.PSD ||
				h.Series.Description != test.Desc || h.Image.TR != test.TR ||
				h.Image.RawRunNum != test.RunNum || h.Exam.MagStrength != 30000 {
				t.Errorf("unexpected header: point_size %d psd %q desc %q TR %d run %d field %d",
					h.RDB.PointSize, h.Image.PSDName, h.Series.Description, h.Image.TR,
					h.Image.RawRunNum, h.Exam.MagStrength)
			}
			checkSampleBaselines(t, f)
		})
	}
}

// checkSampleBaselines checks that every block starts with an all-zero
// baseline view and that the last block ends exactly at end of file.
func checkSampleBaselines(t *testing.T, f *File) {
	t.Helper()
	d := f.Dims()
	view := make([]byte, d.Points*2*int(f.Header.RDB.PointSize))
	zero := make([]byte, len(view))
	for r := range d.Receivers {
		for sl := range d.Slices {
			off, err := f.BlockOffset(r, sl, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := readAt(f.r, view, off); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(view, zero) {
				t.Fatalf("baseline view of block (%d,%d) is not zero", r, sl)
			}
		}
	}
	last, err := f.BlockOffset(d.Receivers-1, d.Slices-1, d.Echoes-1)
	if err != nil {
		t.Fatal(err)
	}
	if end := last + f.blockBytes(); end != f.size {
		t.Errorf("last block ends at %d, file size %d", end, f.size)
	}
}
