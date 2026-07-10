package hftolocal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	ggufTypeUint8   uint32 = 0
	ggufTypeInt8    uint32 = 1
	ggufTypeUint16  uint32 = 2
	ggufTypeInt16   uint32 = 3
	ggufTypeUint32  uint32 = 4
	ggufTypeInt32   uint32 = 5
	ggufTypeFloat32 uint32 = 6
	ggufTypeBool    uint32 = 7
	ggufTypeString  uint32 = 8
	ggufTypeArray   uint32 = 9
	ggufTypeUint64  uint32 = 10
	ggufTypeInt64   uint32 = 11
	ggufTypeFloat64 uint32 = 12

	maxGGUFStringLength = 64 << 20
)

var errCompatibleGGUFFound = errors.New("compatible GGUF found")

type GGUFMetadata struct {
	Architecture string
	Type         string
}

type ggufClassification struct {
	metadata   GGUFMetadata
	compatible bool
}

type ggufClassifier struct {
	results map[string]ggufClassification
}

func newGGUFClassifier() *ggufClassifier {
	return &ggufClassifier{results: map[string]ggufClassification{}}
}

var llamaCppGenerationArchitectures = map[string]struct{}{
	"afmoe":            {},
	"apertus":          {},
	"arcee":            {},
	"arctic":           {},
	"arwkv7":           {},
	"baichuan":         {},
	"bailingmoe":       {},
	"bailingmoe2":      {},
	"bitnet":           {},
	"bloom":            {},
	"chameleon":        {},
	"chatglm":          {},
	"codeshell":        {},
	"cohere2":          {},
	"cohere2moe":       {},
	"command-r":        {},
	"cogvlm":           {},
	"dbrx":             {},
	"deci":             {},
	"deepseek":         {},
	"deepseek2":        {},
	"deepseek32":       {},
	"deepseek4":        {},
	"dflash":           {},
	"dots1":            {},
	"dream":            {},
	"eagle3":           {},
	"ernie4_5":         {},
	"ernie4_5-moe":     {},
	"exaone":           {},
	"exaone-moe":       {},
	"exaone4":          {},
	"falcon":           {},
	"falcon-h1":        {},
	"gemma":            {},
	"gemma2":           {},
	"gemma3":           {},
	"gemma3n":          {},
	"gemma4":           {},
	"gemma4-assistant": {},
	"glm-dsa":          {},
	"glm4":             {},
	"glm4moe":          {},
	"gpt-oss":          {},
	"gpt2":             {},
	"gptj":             {},
	"gptneox":          {},
	"granite":          {},
	"granitehybrid":    {},
	"granitemoe":       {},
	"grok":             {},
	"grovemoe":         {},
	"hunyuan-dense":    {},
	"hunyuan-moe":      {},
	"hunyuan_vl":       {},
	"internlm2":        {},
	"jais":             {},
	"jais2":            {},
	"jamba":            {},
	"kimi-linear":      {},
	"lfm2":             {},
	"lfm2moe":          {},
	"llama":            {},
	"llama4":           {},
	"llada":            {},
	"llada-moe":        {},
	"maincoder":        {},
	"mamba":            {},
	"mamba2":           {},
	"mellum":           {},
	"mimo2":            {},
	"minicpm":          {},
	"minicpm3":         {},
	"minimax-m2":       {},
	"mistral3":         {},
	"mistral4":         {},
	"mpt":              {},
	"nemotron":         {},
	"nemotron_h":       {},
	"nemotron_h_moe":   {},
	"olmo":             {},
	"olmo2":            {},
	"olmoe":            {},
	"openelm":          {},
	"orion":            {},
	"phi2":             {},
	"phi3":             {},
	"phimoe":           {},
	"plamo":            {},
	"plamo2":           {},
	"plamo3":           {},
	"plm":              {},
	"qwen":             {},
	"qwen2":            {},
	"qwen2moe":         {},
	"qwen2vl":          {},
	"qwen3":            {},
	"qwen35":           {},
	"qwen35moe":        {},
	"qwen3moe":         {},
	"qwen3next":        {},
	"qwen3vl":          {},
	"qwen3vlmoe":       {},
	"refact":           {},
	"rwkv6":            {},
	"rwkv6qwen2":       {},
	"rwkv7":            {},
	"rnd1":             {},
	"seed_oss":         {},
	"smallthinker":     {},
	"smollm3":          {},
	"stablelm":         {},
	"starcoder":        {},
	"starcoder2":       {},
	"step35":           {},
	"t5":               {},
	"talkie":           {},
	"xverse":           {},
}

func (classifier *ggufClassifier) shouldIncludeGGUF(path string, llamaCppOnly bool) bool {
	if !isGGUF(path) {
		return false
	}
	if isCompanionGGUF(path) {
		return true
	}
	return classifier.shouldIncludePrimaryGGUF(path, llamaCppOnly)
}

func (classifier *ggufClassifier) shouldIncludePrimaryGGUF(path string, llamaCppOnly bool) bool {
	if !isGGUF(path) || isCompanionGGUF(path) {
		return false
	}
	if !llamaCppOnly {
		return true
	}
	_, compatible := classifier.classifyGGUF(path)
	return compatible
}

func isCompanionGGUF(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(name, "mmproj-") && strings.HasSuffix(name, ".gguf") ||
		strings.HasPrefix(name, "mtp-") && strings.HasSuffix(name, ".gguf")
}

func snapshotHasCompatibleGGUF(snapshot Snapshot, classifier *ggufClassifier) (bool, error) {
	compatible := false
	err := walkFiles(snapshot.Path, func(source string) error {
		if classifier.shouldIncludePrimaryGGUF(source, true) {
			compatible = true
			return errCompatibleGGUFFound
		}
		return nil
	})
	if errors.Is(err, errCompatibleGGUFFound) {
		return true, nil
	}
	return compatible, err
}

func (classifier *ggufClassifier) classifyGGUF(path string) (GGUFMetadata, bool) {
	if result, ok := classifier.results[path]; ok {
		return result.metadata, result.compatible
	}

	metadata, err := readGGUFMetadata(path)
	if err != nil {
		classifier.results[path] = ggufClassification{}
		return GGUFMetadata{}, false
	}
	compatible := isLlamaCppGenerationMetadata(metadata)
	classifier.results[path] = ggufClassification{metadata: metadata, compatible: compatible}
	return metadata, compatible
}

func isLlamaCppGenerationMetadata(metadata GGUFMetadata) bool {
	architecture := strings.ToLower(strings.TrimSpace(metadata.Architecture))
	if _, ok := llamaCppGenerationArchitectures[architecture]; !ok {
		return false
	}
	modelType := strings.ToLower(strings.TrimSpace(metadata.Type))
	return modelType == "" || modelType == "model" || modelType == "language model"
}

func readGGUFMetadata(path string) (GGUFMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return GGUFMetadata{}, err
	}
	defer file.Close()

	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return GGUFMetadata{}, err
	}
	if string(magic[:]) != "GGUF" {
		return GGUFMetadata{}, fmt.Errorf("%s is not a GGUF file", path)
	}

	var version uint32
	if err := binary.Read(file, binary.LittleEndian, &version); err != nil {
		return GGUFMetadata{}, err
	}
	if version == 0 || version > 3 {
		return GGUFMetadata{}, fmt.Errorf("unsupported GGUF version %d", version)
	}

	var tensorCount uint64
	var metadataCount uint64
	if err := binary.Read(file, binary.LittleEndian, &tensorCount); err != nil {
		return GGUFMetadata{}, err
	}
	if err := binary.Read(file, binary.LittleEndian, &metadataCount); err != nil {
		return GGUFMetadata{}, err
	}

	var metadata GGUFMetadata
	for i := uint64(0); i < metadataCount; i++ {
		key, err := readGGUFString(file)
		if err != nil {
			return GGUFMetadata{}, err
		}

		var valueType uint32
		if err := binary.Read(file, binary.LittleEndian, &valueType); err != nil {
			return GGUFMetadata{}, err
		}

		if key == "general.architecture" && valueType == ggufTypeString {
			metadata.Architecture, err = readGGUFString(file)
		} else if key == "general.type" && valueType == ggufTypeString {
			metadata.Type, err = readGGUFString(file)
		} else {
			err = skipGGUFValue(file, valueType)
		}
		if err != nil {
			return GGUFMetadata{}, err
		}
	}

	if metadata.Architecture == "" {
		return metadata, fmt.Errorf("GGUF metadata missing general.architecture")
	}
	return metadata, nil
}

func readGGUFString(reader io.Reader) (string, error) {
	var length uint64
	if err := binary.Read(reader, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length > maxGGUFStringLength {
		return "", fmt.Errorf("GGUF string length %d exceeds limit", length)
	}

	buffer := make([]byte, length)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return string(buffer), nil
}

func skipGGUFValue(seeker io.ReadSeeker, valueType uint32) error {
	switch valueType {
	case ggufTypeUint8, ggufTypeInt8, ggufTypeBool:
		return seekGGUFBytes(seeker, 1)
	case ggufTypeUint16, ggufTypeInt16:
		return seekGGUFBytes(seeker, 2)
	case ggufTypeUint32, ggufTypeInt32, ggufTypeFloat32:
		return seekGGUFBytes(seeker, 4)
	case ggufTypeUint64, ggufTypeInt64, ggufTypeFloat64:
		return seekGGUFBytes(seeker, 8)
	case ggufTypeString:
		_, err := readGGUFString(seeker)
		return err
	case ggufTypeArray:
		return skipGGUFArray(seeker)
	default:
		return fmt.Errorf("unsupported GGUF metadata type %d", valueType)
	}
}

func skipGGUFArray(seeker io.ReadSeeker) error {
	var elementType uint32
	if err := binary.Read(seeker, binary.LittleEndian, &elementType); err != nil {
		return err
	}
	var length uint64
	if err := binary.Read(seeker, binary.LittleEndian, &length); err != nil {
		return err
	}

	elementSize, ok := fixedGGUFTypeSize(elementType)
	if ok {
		return seekGGUFBytes(seeker, int64(length)*elementSize)
	}
	for i := uint64(0); i < length; i++ {
		if err := skipGGUFValue(seeker, elementType); err != nil {
			return err
		}
	}
	return nil
}

func fixedGGUFTypeSize(valueType uint32) (int64, bool) {
	switch valueType {
	case ggufTypeUint8, ggufTypeInt8, ggufTypeBool:
		return 1, true
	case ggufTypeUint16, ggufTypeInt16:
		return 2, true
	case ggufTypeUint32, ggufTypeInt32, ggufTypeFloat32:
		return 4, true
	case ggufTypeUint64, ggufTypeInt64, ggufTypeFloat64:
		return 8, true
	default:
		return 0, false
	}
}

func seekGGUFBytes(seeker io.Seeker, offset int64) error {
	_, err := seeker.Seek(offset, io.SeekCurrent)
	return err
}
