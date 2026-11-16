// Command pfinfo prints the header of a GE P-file. It can also write a
// rough magnitude image per slice as PNG.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"

	"github.com/ripta/ge-p-rdb/pkg/pfile"
)

func main() {
	asJSON := flag.Bool("json", false, "print the full decoded header as JSON")
	pngDir := flag.String("png", "", "write a sum-of-squares PNG per slice into this directory")
	blocks := flag.Bool("blocks", false, "with -png, also write one PNG per receiver, slice and echo")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pfinfo [-json] [-png dir [-blocks]] Pxxxxx.7")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *asJSON, *pngDir, *blocks); err != nil {
		fmt.Fprintln(os.Stderr, "pfinfo:", err)
		os.Exit(1)
	}
}

func run(path string, asJSON bool, pngDir string, blocks bool) error {
	f, err := pfile.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(f.Header); err != nil {
			return err
		}
	} else {
		summary(f)
	}

	if pngDir == "" {
		return nil
	}
	if err := f.DataErr(); err != nil {
		return err
	}
	return writePNGs(f, pngDir, blocks)
}

func summary(f *pfile.File) {
	h := &f.Header
	r := &h.RDB
	d := f.Dims()
	fmt.Printf("rdbm_rev      %.3f\n", r.Revision)
	fmt.Printf("scan          %s %s\n", r.ScanDate, r.ScanTime)
	fmt.Printf("exam/series   %d / %d  %q\n", h.Exam.ExamNumber, h.Series.SeriesNo, h.Series.Description)
	fmt.Printf("site          %q (%s)\n", h.Exam.HospName, h.Exam.SysID)
	fmt.Printf("field         %.1f T\n", float64(h.Exam.MagStrength)/10000)
	fmt.Printf("psd           %q coil %q\n", h.Image.PSDName, h.Image.CoilName)
	fmt.Printf("TR/TE/TI      %.1f / %.2f / %.1f ms, flip %d°\n",
		float64(h.Image.TR)/1000, float64(h.Image.TE)/1000, float64(h.Image.TI)/1000, h.Image.FlipAngle)
	fmt.Printf("FOV           %.1f mm\n", h.Image.DFOV)
	fmt.Printf("dims          %d receivers × %d slices × %d echoes × %d views × %d points\n",
		d.Receivers, d.Slices, d.Echoes, d.Views, d.Points)
	fmt.Printf("point size    %d bytes (int%d)\n", r.PointSize, 8*r.PointSize)
	fmt.Printf("data          offset %d, %d bytes per pass, %d passes\n", r.Offsets.Data, r.RawPassSize, r.NPasses)
	if err := f.DataErr(); err != nil {
		fmt.Printf("data error    %v\n", err)
	}
	fmt.Println("slices (spatial order):")
	for i, e := range h.AcqTable {
		fmt.Printf("  %3d  pass %d acq %3d  gw_point1 (%.2f, %.2f, %.2f)\n",
			i, e.PassNumber, e.SliceInPass, e.GWPoint1[0], e.GWPoint1[1], e.GWPoint1[2])
	}
}

// writePNGs writes one sum-of-squares image per spatial slice and echo.
func writePNGs(f *pfile.File, dir string, blocks bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	d := f.Dims()
	for spatial := range f.Header.AcqTable {
		acq, err := f.AcqSlice(spatial)
		if err != nil {
			return err
		}
		for echo := range d.Echoes {
			sos := make([]float64, d.Views*d.Points)
			for rcv := range d.Receivers {
				b, err := f.ReadBlock(rcv, acq, echo)
				if err != nil {
					return err
				}
				img := idft2(b)
				for i, v := range img {
					sos[i] += v * v
				}
				if blocks {
					name := fmt.Sprintf("block_r%02d_a%03d_e%02d.png", rcv, acq, echo)
					if err := savePNG(filepath.Join(dir, name), img, d.Views, d.Points); err != nil {
						return err
					}
				}
			}
			for i := range sos {
				sos[i] = math.Sqrt(sos[i])
			}
			name := fmt.Sprintf("slice_s%03d_e%02d.png", spatial, echo)
			if err := savePNG(filepath.Join(dir, name), sos, d.Views, d.Points); err != nil {
				return err
			}
		}
	}
	return nil
}

// idft2 returns the magnitude of the centered 2D inverse DFT of b. It is a
// plain separable DFT. That is fast enough for typical matrix sizes.
func idft2(b *pfile.Block) []float64 {
	ny, nx := b.Views, b.Points
	buf := make([]complex128, ny*nx)
	for y := range ny {
		for x := range nx {
			v := b.At(y, x)
			if (x+y)%2 == 1 {
				// Shift the k-space origin to the center.
				v = -v
			}
			buf[y*nx+x] = v
		}
	}
	tmp := make([]complex128, max(nx, ny))
	row := make([]complex128, max(nx, ny))
	for y := range ny {
		copy(row, buf[y*nx:(y+1)*nx])
		dft(tmp[:nx], row[:nx])
		copy(buf[y*nx:], tmp[:nx])
	}
	for x := range nx {
		for y := range ny {
			row[y] = buf[y*nx+x]
		}
		dft(tmp[:ny], row[:ny])
		for y := range ny {
			buf[y*nx+x] = tmp[y]
		}
	}
	out := make([]float64, len(buf))
	for i, v := range buf {
		out[i] = cmplx.Abs(v)
	}
	return out
}

var twiddles = map[int][]complex128{}

// dft computes an inverse DFT without normalization.
func dft(dst, src []complex128) {
	n := len(src)
	w, ok := twiddles[n]
	if !ok {
		w = make([]complex128, n)
		for i := range w {
			w[i] = cmplx.Rect(1, 2*math.Pi*float64(i)/float64(n))
		}
		twiddles[n] = w
	}
	for k := range n {
		var s complex128
		for j, v := range src {
			s += v * w[j*k%n]
		}
		dst[k] = s
	}
}

func savePNG(path string, img []float64, h, w int) error {
	var peak float64
	for _, v := range img {
		peak = max(peak, v)
	}
	g := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v := 0.0
			if peak > 0 {
				v = img[y*w+x] / peak
			}
			g.SetGray(x, y, color.Gray{Y: uint8(math.Round(255 * math.Min(1, v)))})
		}
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(fh, g); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}
