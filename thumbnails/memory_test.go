package thumbnails_test

import (
	"bytes"
	"compress/gzip"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"runtime"
	"runtime/metrics"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/thumbnails"
)

// heap is the bytes the heap's live and unswept objects take.
func heap() uint64 {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(sample)
	return sample[0].Value.Uint64()
}

// peakDuring is the most the heap grew while run ran, sampled every half millisecond.
func peakDuring(run func()) uint64 {
	runtime.GC()
	base := heap()
	var peak atomic.Uint64
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				if grown := heap() - min(base, heap()); grown > peak.Load() {
					peak.Store(grown)
				}
				time.Sleep(500 * time.Microsecond)
			}
		}
	}()
	run()
	close(done)
	return peak.Load()
}

// TestATallPictureStaysWithinItsAdmittedMemory is the review's picture: a 601×26622 PNG of under a hundred
// kilobytes, which once grew the heap by 611 MiB in the card preset, as CatmullRom kept 600 × 26622 float64 pixels.
func TestATallPictureStaysWithinItsAdmittedMemory(t *testing.T) {
	tall := image.NewNRGBA(image.Rect(0, 0, 601, 26622))
	for y := range 26622 {
		for x := range 601 {
			tall.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var file bytes.Buffer
	require.NoError(t, png.Encode(&file, tall))
	tall = nil //nolint:wastedassign // freed before the heap is measured
	box := thumbnails.Box{Width: 600, Mode: thumbnails.Fit}
	cost, err := thumbnails.Cost(file.Bytes(), box, 16_000_000)
	require.NoError(t, err)

	width, height := thumbnails.Scaled(601, 26622, box)
	assert.LessOrEqual(t, height, 3*(width+1), "within the aspect, give or take rounding")
	assert.Less(t, cost, int64(128<<20))
	for _, format := range []thumbnails.Format{thumbnails.Classic, thumbnails.WebP} {
		var made image.Point
		peak := peakDuring(func() {
			result, err := thumbnails.Resize(file.Bytes(), box, 16_000_000, format)
			require.NoError(t, err)
			made = image.Pt(result.Width, result.Height)
		})

		t.Logf("%q: %dx%d, cost %d MiB, peak %d MiB", format, width, height, cost>>20, peak>>20)
		assert.Equal(t, image.Pt(width, height), made)
		assert.LessOrEqual(t, peak, uint64(cost)+16<<20, "peak %d MiB against a cost of %d MiB", peak>>20, cost>>20)
	}
}

// progressive is a 4000×4000 JPEG coded in successive scans without chroma subsampling, of one flat grey.
func progressive(t *testing.T) []byte {
	t.Helper()
	file, err := os.Open("testdata/progressive-444.jpg.gz")
	require.NoError(t, err)
	defer file.Close()
	unzipped, err := gzip.NewReader(file)
	require.NoError(t, err)
	body, err := io.ReadAll(unzipped)
	require.NoError(t, err)
	return body
}

// TestAProgressiveJPEGIsChargedItsCoefficients: image/jpeg keeps 256 bytes for each 8×8 block of each component until
// the last scan, 192 MB for this picture, on top of the pixels a sequential JPEG of its size is charged.
func TestAProgressiveJPEGIsChargedItsCoefficients(t *testing.T) {
	file := progressive(t)
	var baseline bytes.Buffer
	require.NoError(t, jpeg.Encode(&baseline, image.NewYCbCr(image.Rect(0, 0, 4000, 4000), image.YCbCrSubsampleRatio420), nil))
	box := thumbnails.Box{Width: 600, Mode: thumbnails.Fit}
	cost, err := thumbnails.Cost(file, box, 16_000_000)
	require.NoError(t, err)
	sequential, err := thumbnails.Cost(baseline.Bytes(), box, 16_000_000)
	require.NoError(t, err)

	assert.Equal(t, int64(3*500*500*256), cost-sequential)

	peak := peakDuring(func() {
		_, err := thumbnails.Resize(file, box, 16_000_000, thumbnails.Classic)
		require.NoError(t, err)
	})
	t.Logf("cost %d MiB, peak %d MiB", cost>>20, peak>>20)
	assert.LessOrEqual(t, peak, uint64(cost)+16<<20, "peak %d MiB against a cost of %d MiB", peak>>20, cost>>20)
}
