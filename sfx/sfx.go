// Package sfx — игровые звуки интерфейса и приказов поверх пакета audio.
//
// Два источника звуков:
//
//   - Движковые звуки UI встроены в бинарь (извлечены из Engine.sga):
//     щелчок по кнопке — тот самый taskbar_button_click, который в
//     gamescreen.screen прописан у кнопок как sample = "taskbar_button_click";
//     короткий сигнал ping_cycle — общий отклик приказа, когда у юнита
//     своей реплики нет.
//
//   - Озвучка юнитов читается на лету из дерева мода. Чертёж ebps несёт
//     ui_ext/speech_directory ("Speech/Races/Necron/Builder_Scarab"), а
//     физически реплики лежат в Data/sound/speech/races/<раса>/<юнит>
//     или Data/sound/races/<раса>/<юнит> (в разных модах по-разному).
//     Внутри папки юнита — категории действий: capture_strategic_point,
//     move, flag_plant, die … Это и есть типизированные приказы, те же
//     имена, что в действиях whe/events. Категория и разрешается в
//     конкретные .fda-реплики.
package sfx

import (
	"embed"
	"math/rand"
	"path"
	"strings"
	"sync"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/audio"
	"github.com/Anykey-Nomad/Open-Kaurava/settings"
)

//go:embed sfxdata/*.fda
var embeddedFS embed.FS

// Громкости: голос юнита громче, UI-щелчки и общий сигнал тише.
const (
	clickGain = 0.55
	voiceGain = 0.9
	pingGain  = 0.5
)

// Типизированные приказы — категории в папке озвучки юнита: первая
// найденная и играет. Захват у некронов называется capture_strategic_point,
// общий флаг — flag_plant; движение — move или rally.
var (
	moveCats    = []string{"move", "rally"}
	captureCats = []string{"capture_strategic_point", "capture_here", "capture", "flag_plant"}
	// В DoWDE категории названы по-разному для разных типов реплик:
	// выбор — selection, приказ атаки — attack. Имена даны в порядке
	// предпочтения: первая найденная и играет.
	selectCats   = []string{"selection", "select", "selected"}
	attackCats   = []string{"attack", "combat", "attack_order"}
	fallbackFlag = `Data\sound\races\shared\flag_plant.fda`
)

const (
	embeddedClick = "sfxdata/taskbar_button_click.fda"
	embeddedPing  = "sfxdata/ping_cycle.fda"
)

var (
	cacheMu sync.Mutex
	cache   = map[string]audio.Sample{}
)

// EmbeddedSample возвращает встроенный движковой звук (для проверок и
// встроенного сигнала приказа), уже в формате устройства.
func EmbeddedSample(name string) (audio.Sample, error) {
	return cached(name, func() (audio.Sample, error) {
		data, err := embeddedFS.ReadFile(name)
		if err != nil {
			return audio.Sample{}, err
		}
		return audio.LoadFDA(data)
	})
}

// cached — декодированный сэмпл один раз на источник: повторные щелчки
// и приказы не декодируют FDA заново.
func cached(key string, load func() (audio.Sample, error)) (audio.Sample, error) {
	cacheMu.Lock()
	if s, ok := cache[key]; ok {
		cacheMu.Unlock()
		return s, nil
	}
	cacheMu.Unlock()
	s, err := load()
	if err != nil {
		return audio.Sample{}, err
	}
	cacheMu.Lock()
	cache[key] = s
	cacheMu.Unlock()
	return s, nil
}

// UIClick — щелчок по элементу интерфейса (кнопки меню и HUD). Ставится
// как scene.OnUIClick: один звук за кадр на первом клике ЛКМ по кнопке.
func UIClick() {
	s, err := EmbeddedSample(embeddedClick)
	if err != nil {
		return
	}
	gain := clickGain * settings.Effects()
	_, _ = audio.Play(s, gain, gain)
}

// OrderAck — отклик на принятый приказ движения или захвата: голос юнита
// из его озвучки, а если реплики нет — общий сигнал приказа, чтобы
// подтверждение всегда было слышно.
func OrderAck(tree *modapi.ModTree, speech string, isCapture bool) {
	s, source, ok := Resolve(tree, speech, isCapture)
	if !ok {
		return
	}
	gain := voiceGain * settings.Speech()
	if strings.HasPrefix(source, "embedded:") {
		gain = pingGain * settings.Effects()
	}
	_, _ = audio.Play(s, gain, gain)
}

// Resolve разрешает отклик приказа в готовый сэмпл и его источник
// (путь в дереве мода или имя встроенного звука). Порядок:
//
//  1. папка юнита по speech_directory + категория действия;
//  2. для захвата — общий флаг races/shared/flag_plant.fda;
//  3. встроенный сигнал ping_cycle, чтобы приказ всегда был слышен.
func Resolve(tree *modapi.ModTree, speech string, isCapture bool) (audio.Sample, string, bool) {
	cats := moveCats
	if isCapture {
		cats = captureCats
	}
	if s, key, ok := resolveCategories(tree, speech, cats); ok {
		return s, key, true
	}
	if isCapture {
		if s, ok := resolveFile(tree, fallbackFlag); ok {
			return s, fallbackFlag, true
		}
	}
	if s, err := EmbeddedSample(embeddedPing); err == nil {
		return s, "embedded:" + embeddedPing, true
	}
	return audio.Sample{}, "", false
}

// AttackAck — отклик на приказ атаки: голос юнита, а без реплики — общий
// сигнал приказа, как у движения.
func AttackAck(tree *modapi.ModTree, speech string) {
	if s, _, ok := resolveCategories(tree, speech, attackCats); ok {
		gain := voiceGain * settings.Speech()
		_, _ = audio.Play(s, gain, gain)
		return
	}
	if s, err := EmbeddedSample(embeddedPing); err == nil {
		gain := pingGain * settings.Effects()
		_, _ = audio.Play(s, gain, gain)
	}
}

// SelectVoice — голос выбранного отряда. Сигнала-заглушки здесь нет:
// каждый клик по отряду не должен пищать, если реплики выбора в моде нет.
func SelectVoice(tree *modapi.ModTree, speech string) {
	s, _, ok := resolveCategories(tree, speech, selectCats)
	if !ok {
		return
	}
	gain := voiceGain * settings.Speech()
	_, _ = audio.Play(s, gain, gain)
}

// resolveCategories ищет реплику по категориям действия в папке озвучки
// юнита: «select» — выбор, «attack» — приказ атаки, «move» — движение.
func resolveCategories(tree *modapi.ModTree, speech string, cats []string) (audio.Sample, string, bool) {
	if tree == nil {
		return audio.Sample{}, "", false
	}
	suffix := speechSuffix(speech)
	if suffix == "" {
		return audio.Sample{}, "", false
	}
	// Корни, в которых моды кладут озвучку: с папкой speech и без.
	roots := []string{`Data\sound\speech\races\`, `Data\sound\races\`}
	for _, root := range roots {
		base := root + suffix
		for _, cat := range cats {
			// DoWDE хранит реплику одним файлом категории
			// (selection.con, attack.con, move.con), а часть модов —
			// папкой той же категории с наборами .fda внутри.
			for _, ext := range []string{".con", ".fda"} {
				key := base + `\` + cat + ext
				if s, ok := resolveFileNonEmpty(tree, key); ok {
					return s, key, true
				}
			}
			key := base + `\` + cat
			if s, ok := resolveFolder(tree, key); ok {
				return s, key, true
			}
		}
	}
	return audio.Sample{}, "", false
}

// speechSuffix переводит speech_directory в хвост пути озвучки:
// "Speech/Races/Necron/Builder_Scarab" → "necron\builder_scarab".
// Пустая строка — путь не похож на озвучку, у юнита нет своего голоса.
func speechSuffix(speech string) string {
	s := strings.ToLower(strings.ReplaceAll(speech, `/`, `\`))
	rest, found := strings.CutPrefix(s, `speech\races\`)
	if !found || rest == "" {
		return ""
	}
	return rest
}

// resolveFolder берёт случайный .fda из папки категории действия.
func resolveFolder(tree *modapi.ModTree, folder string) (audio.Sample, bool) {
	var fda []*modapi.TreeFile
	for _, f := range modapi.FilesInFolder(tree, folder) {
		if strings.EqualFold(path.Ext(f.Name), ".fda") {
			fda = append(fda, f)
		}
	}
	if len(fda) == 0 {
		return audio.Sample{}, false
	}
	return resolveFile2(tree, fda[rand.Intn(len(fda))])
}

// resolveFileNonEmpty — файл реплики по точному пути. Пустые файлы в
// установке DoWDE встречаются (заглушки языковых архивов): такой файл не
// считается найденным, иначе вместо голоса была бы тишина.
func resolveFileNonEmpty(tree *modapi.ModTree, virtualPath string) (audio.Sample, bool) {
	file := tree.FindFile(virtualPath)
	if file == nil {
		return audio.Sample{}, false
	}
	data, err := tree.ReadTreeFile(file)
	if err != nil || len(data) == 0 {
		return audio.Sample{}, false
	}
	sample, err := cached("tree:"+strings.ToLower(file.FullPath), func() (audio.Sample, error) {
		return audio.LoadFDA(data)
	})
	if err != nil || len(sample.PCM) == 0 {
		return audio.Sample{}, false
	}
	return sample, true
}

// resolveFile — .fda по точному пути в дереве мода.
func resolveFile(tree *modapi.ModTree, virtualPath string) (audio.Sample, bool) {
	f := tree.FindFile(virtualPath)
	if f == nil {
		return audio.Sample{}, false
	}
	return resolveFile2(tree, f)
}

// resolveFile2 — читает файл дерева и декодирует в формат устройства.
func resolveFile2(tree *modapi.ModTree, f *modapi.TreeFile) (audio.Sample, bool) {
	key := "tree:" + strings.ToLower(f.FullPath)
	s, err := cached(key, func() (audio.Sample, error) {
		data, err := tree.ReadTreeFile(f)
		if err != nil {
			return audio.Sample{}, err
		}
		return audio.LoadFDA(data)
	})
	if err != nil {
		return audio.Sample{}, false
	}
	return s, true
}
