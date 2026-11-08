package audio

import (
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"
)

// GainReader — поток PCM в формате устройства, который перед каждым кадром
// умножает оба канала на текущие множители громкости. Пара (left, right)
// и есть задел под объёмный звук: на стерео пока этого достаточно, а
// позиционирование подставит два числа каждый кадр, ничего не меняя в
// устройстве и формате.
//
// Множители читаются через atomic — писать их может другой поток (кадр
// игры), читает поток звука, гонка данных здесь недопустима.
type GainReader struct {
	// pcm кадры s16LE в формате устройства; данные только читаются.
	pcm []byte
	// pos — байтовая позиция чтения: кадр всегда 4 байта, выравненный,
	// поэтому и позиция всегда выровнена.
	pos int
	// left и right — множители громкости 0..N: 1.0 полная, 0.0 тишина.
	// Отрицательные значения обнуляются.
	left, right atomic.Uint32
	// done закрывается один раз, когда чтение дошло до конца сэмпла:
	// по нему голос узнаёт, что пора освободить плеер.
	done     chan struct{}
	doneOnce sync.Once
}

// NewGainReader создаёт поток с заданной парой громкостей.
func NewGainReader(pcm []byte, left, right float64) *GainReader {
	r := &GainReader{pcm: pcm, done: make(chan struct{})}
	r.SetVolumes(left, right)
	return r
}

// SetVolumes задаёт громкости каналов (float bits в atomic — атомарно и
// без аллокаций). Вызывается из любого потока во время звучания.
func (r *GainReader) SetVolumes(left, right float64) {
	if left < 0 {
		left = 0
	}
	if right < 0 {
		right = 0
	}
	r.left.Store(float32bits(float32(left)))
	r.right.Store(float32bits(float32(right)))
}

// Volumes возвращает текущую пару громкостей (для отладки/тестов).
func (r *GainReader) Volumes() (left, right float64) {
	return float64(float32frombits(r.left.Load())), float64(float32frombits(r.right.Load()))
}

// Read отдаёт плееру следующий кадр с применёнными громкостями: пока
// плеер не считал текущий кадр, он не считает и следующий, так что
// смена громкости применяется без щелчка — мгновенно, но с границы
// кадра (0.02 мс при 48 кГц).
func (r *GainReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.pcm) {
		// Конец сэмпла: io.EOF останавливает чтение — плеер доигрывает
		// уже отданные буферы и завершается сам. Без EOF плеер крутит
		// цикл на нулевых чтениях.
		r.doneOnce.Do(func() { close(r.done) })
		return 0, io.EOF
	}
	n := copy(p, r.pcm[r.pos:])
	left := float32frombits(r.left.Load())
	right := float32frombits(r.right.Load())
	for i := 0; i+3 < n; i += 4 {
		l := int32(int16(uint16(p[i]) | uint16(p[i+1])<<8))
		rr := int32(int16(uint16(p[i+2]) | uint16(p[i+3])<<8))
		l = int32(float32(l) * left)
		rr = int32(float32(rr) * right)
		l = clamp32(l)
		rr = clamp32(rr)
		p[i] = byte(l)
		p[i+1] = byte(l >> 8)
		p[i+2] = byte(rr)
		p[i+3] = byte(rr >> 8)
	}
	r.pos += n
	return n, nil
}

func clamp32(v int32) int32 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return v
}

// Перенос float32 в атомарное uint32 и обратно: atomic-операции работают
// с целыми, а math.Float32bits — чистая битовая перекладовка.
func float32bits(f float32) uint32     { return math.Float32bits(f) }
func float32frombits(b uint32) float32 { return math.Float32frombits(b) }

// Voice — ручка одного звучащего сэмпла: параллельного с остальными.
// Все методы потокобезопасны и могут вызываться во время звучания.
type Voice struct {
	reader *GainReader
	player *oto.Player

	mu     sync.Mutex
	closed bool
	// stopCh закрывается в Stop: вспомогательный поток прекращает
	// ожидание конца сэмпла, если голос остановили раньше.
	stopCh chan struct{}
}

// SetVolumes меняет громкость канала на лету: 1.0 полная, 0.0 тишина.
func (v *Voice) SetVolumes(left, right float64) {
	v.reader.SetVolumes(left, right)
}

// Stop останавливает звук и освобождает плеер. Повторный вызов безопасен.
func (v *Voice) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return
	}
	v.closed = true
	close(v.stopCh)
	v.player.Pause()
	v.player.Close()
}

// retire дожидается конца сэмпла и освобождает плеер сам: игровые звуки
// (щелчки, приказы) играют без ручки Stop, и без этого плееры копились
// бы за сессию. После io.EOF плееру отдаётся пауза на последний буфер.
func (v *Voice) retire() {
	select {
	case <-v.reader.done:
		time.Sleep(750 * time.Millisecond)
		v.Stop()
	case <-v.stopCh:
	}
}

// Playing сообщает, идёт ли воспроизведение (сэмпл не кончился и не был
// остановлен). По завершении плеер сам переходит в это состояние.
func (v *Voice) Playing() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return !v.closed && v.reader.pos < len(v.reader.pcm)
}
