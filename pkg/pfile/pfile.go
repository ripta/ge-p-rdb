package pfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
)

// ErrUnsupportedRevision is returned for an rdbm_rev this package cannot
// decode.
var ErrUnsupportedRevision = errors.New("pfile: unsupported rdbm_rev")

// File is an open P-file.
type File struct {
	Header Header

	r       io.ReaderAt
	closer  io.Closer
	size    int64
	dims    Dims
	dataErr error
}

// Dims gives the shape of the sample data.
type Dims struct {
	Receivers int
	// Slices are indexed in acquisition order.
	Slices int
	Echoes int
	// Views is the phase-encode views per block, without the baseline view.
	Views int
	// Points is the complex samples per view.
	Points int
}

// Open opens and decodes the header of the P-file at path.
func Open(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := fh.Stat()
	if err != nil {
		fh.Close()
		return nil, err
	}
	f, err := New(fh, st.Size())
	if err != nil {
		fh.Close()
		return nil, err
	}
	f.closer = fh
	return f, nil
}

// New decodes the header of a P-file of the given size read from r.
//
// The header must be self-consistent or New returns an error. Problems with
// the sample data, such as a truncated file, do not fail New. They are
// reported by DataErr and by the methods that read samples.
func New(r io.ReaderAt, size int64) (*File, error) {
	if size < rdbHeaderMinSize {
		return nil, fmt.Errorf("pfile: file too small (%d bytes)", size)
	}
	pre := make([]byte, rdbHeaderMinSize)
	if err := readAt(r, pre, 0); err != nil {
		return nil, fmt.Errorf("pfile: reading header: %w", err)
	}
	rev := math.Float32frombits(binary.LittleEndian.Uint32(pre))
	if math.IsNaN(float64(rev)) || rev < 26 || rev >= 27 {
		return nil, fmt.Errorf("%w %.3f (only 26.x is supported)", ErrUnsupportedRevision, rev)
	}

	offData := int64(int32(binary.LittleEndian.Uint32(pre[offOffData:])))
	if offData < rdbHeaderMinSize || offData > size {
		return nil, fmt.Errorf("pfile: off_data %d out of range for file size %d", offData, size)
	}
	hb := make([]byte, offData)
	if err := readAt(r, hb, 0); err != nil {
		return nil, fmt.Errorf("pfile: reading header: %w", err)
	}

	rdb := parseRDB(sec{b: hb})
	if err := checkRDB(&rdb); err != nil {
		return nil, err
	}
	o := rdb.Offsets

	exam, err := section(hb, o, o.Exam, examMinSize, "exam")
	if err != nil {
		return nil, err
	}
	series, err := section(hb, o, o.Series, seriesMinSize, "series")
	if err != nil {
		return nil, err
	}
	image, err := section(hb, o, o.Image, imageMinSize, "image")
	if err != nil {
		return nil, err
	}
	acq, err := section(hb, o, o.DataAcqTab, int(rdb.NSlices)*acqEntrySize, "data_acq_tab")
	if err != nil {
		return nil, err
	}

	f := &File{
		Header: Header{
			RDB:      rdb,
			Exam:     parseExam(exam),
			Series:   parseSeries(series),
			Image:    parseImage(image),
			AcqTable: parseAcqTable(acq, int(rdb.NSlices)),
		},
		r:    r,
		size: size,
		dims: Dims{
			Receivers: rdb.NumReceivers(),
			Slices:    int(rdb.NSlices),
			Echoes:    int(rdb.NEchoes),
			Views:     int(rdb.NFrames),
			Points:    int(rdb.FrameSize),
		},
	}
	f.dataErr = f.checkData()
	return f, nil
}

func checkRDB(h *RDBHeader) error {
	switch {
	case h.PointSize != 2 && h.PointSize != 4:
		return fmt.Errorf("pfile: point_size %d is not 2 or 4", h.PointSize)
	case h.NSlices <= 0:
		return fmt.Errorf("pfile: nslices %d is not positive", h.NSlices)
	case h.NEchoes <= 0:
		return fmt.Errorf("pfile: nechoes %d is not positive", h.NEchoes)
	case h.NFrames <= 0:
		return fmt.Errorf("pfile: nframes %d is not positive", h.NFrames)
	case h.FrameSize == 0:
		return errors.New("pfile: frame_size is zero")
	case h.NumReceivers() <= 0:
		return fmt.Errorf("pfile: dab[0] receiver range %d..%d is empty", h.DAB[0].StartRcv, h.DAB[0].StopRcv)
	case h.RawPassSize < 0:
		return fmt.Errorf("pfile: raw_pass_size %d is negative", h.RawPassSize)
	}
	return nil
}

// section returns the bytes of the section at start. It checks that need
// bytes fit before the next section pointer, or before off_data.
func section(hb []byte, o Offsets, start int32, need int, name string) (sec, error) {
	ptrs := []int32{o.Data, o.PerPass, o.UnlockRaw, o.DataAcqTab, o.NexTab, o.NexAbortTab,
		o.Tool, o.Exam, o.Series, o.Image, o.PS, o.GradData, o.CTTData}
	slices.Sort(ptrs)
	end := o.Data
	for _, p := range ptrs {
		if p > start {
			end = min(end, p)
			break
		}
	}
	if start < rdbHeaderMinSize || int64(start)+int64(need) > int64(end) {
		return sec{}, fmt.Errorf("pfile: %s section at %d needs %d bytes, but only %d are available",
			name, start, need, int64(end)-int64(start))
	}
	return sec{b: hb[start:end]}, nil
}

func (f *File) blockBytes() int64 {
	return int64(f.dims.Views+1) * int64(f.dims.Points) * 2 * int64(f.Header.RDB.PointSize)
}

func (f *File) checkData() error {
	h := &f.Header.RDB
	if h.NPasses != 1 {
		return fmt.Errorf("pfile: npasses is %d; only single-pass data layout is supported", h.NPasses)
	}
	want := int64(f.dims.Receivers) * int64(f.dims.Slices) * int64(f.dims.Echoes) * f.blockBytes()
	if want != h.RawPassSize {
		return fmt.Errorf("pfile: header implies %d data bytes, but raw_pass_size is %d", want, h.RawPassSize)
	}
	if have := f.size - int64(h.Offsets.Data); have < want {
		return fmt.Errorf("pfile: file holds %d data bytes, want %d (truncated?)", have, want)
	}
	return nil
}

// Close closes the underlying file if it was opened by Open.
func (f *File) Close() error {
	if f.closer == nil {
		return nil
	}
	return f.closer.Close()
}

// Dims returns the shape of the sample data.
func (f *File) Dims() Dims { return f.dims }

// DataErr reports why the sample data cannot be read, or nil if it can.
func (f *File) DataErr() error { return f.dataErr }

// BlockOffset returns the file offset of the (receiver, slice, echo) block.
// The block starts with the baseline view. Slice is in acquisition order.
func (f *File) BlockOffset(receiver, slice, echo int) (int64, error) {
	if f.dataErr != nil {
		return 0, f.dataErr
	}
	d := f.dims
	if receiver < 0 || receiver >= d.Receivers || slice < 0 || slice >= d.Slices || echo < 0 || echo >= d.Echoes {
		return 0, fmt.Errorf("pfile: block (receiver %d, slice %d, echo %d) out of range %+v", receiver, slice, echo, d)
	}
	idx := (int64(receiver)*int64(d.Slices)+int64(slice))*int64(d.Echoes) + int64(echo)
	return int64(f.Header.RDB.Offsets.Data) + idx*f.blockBytes(), nil
}

// Block is one receiver's k-space for one slice and echo.
type Block struct {
	Views  int
	Points int
	Data   []complex128 // Views*Points samples, view-major
}

// At returns the sample at the given view and point.
func (b *Block) At(view, point int) complex128 {
	return b.Data[view*b.Points+point]
}

// ReadBlock reads the k-space for one receiver, slice and echo. The baseline
// view is dropped. Slice is in acquisition order.
func (f *File) ReadBlock(receiver, slice, echo int) (*Block, error) {
	off, err := f.BlockOffset(receiver, slice, echo)
	if err != nil {
		return nil, err
	}
	ps := int64(f.Header.RDB.PointSize)
	viewBytes := int64(f.dims.Points) * 2 * ps
	buf := make([]byte, f.blockBytes()-viewBytes)
	if err := readAt(f.r, buf, off+viewBytes); err != nil {
		return nil, fmt.Errorf("pfile: reading block: %w", err)
	}
	n := f.dims.Views * f.dims.Points
	data := make([]complex128, n)
	le := binary.LittleEndian
	if ps == 2 {
		for i := range data {
			re := int16(le.Uint16(buf[4*i:]))
			im := int16(le.Uint16(buf[4*i+2:]))
			data[i] = complex(float64(re), float64(im))
		}
	} else {
		for i := range data {
			re := int32(le.Uint32(buf[8*i:]))
			im := int32(le.Uint32(buf[8*i+4:]))
			data[i] = complex(float64(re), float64(im))
		}
	}
	return &Block{Views: f.dims.Views, Points: f.dims.Points, Data: data}, nil
}

// AcqSlice returns the acquisition-order slice index for a spatial slice.
// The spatial index is a position in Header.AcqTable. The result can be
// passed to ReadBlock and BlockOffset.
func (f *File) AcqSlice(spatial int) (int, error) {
	t := f.Header.AcqTable
	if spatial < 0 || spatial >= len(t) {
		return 0, fmt.Errorf("pfile: spatial slice %d out of range [0, %d)", spatial, len(t))
	}
	acq := int(t[spatial].SliceInPass) - 1
	if acq < 0 || acq >= f.dims.Slices {
		return 0, fmt.Errorf("pfile: spatial slice %d has slice_in_pass %d, outside 1..%d", spatial, acq+1, f.dims.Slices)
	}
	return acq, nil
}

// readAt fills p from r at off. The io.ReaderAt contract allows io.EOF
// alongside a full read at the end of the source, so that is not an error.
func readAt(r io.ReaderAt, p []byte, off int64) error {
	n, err := r.ReadAt(p, off)
	if n == len(p) {
		return nil
	}
	return err
}
