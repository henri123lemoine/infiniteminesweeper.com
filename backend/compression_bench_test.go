package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/pierrec/lz4/v4"
)

const (
	TotalCells    = ChunkSize * ChunkSize
	RawChunkBytes = TotalCells / 8
)

// set toggles a cell's state to 'on' (revealed).
func (c *ChunkBits) set(x, y int) {
	if x < 0 || x >= ChunkSize || y < 0 || y >= ChunkSize {
		return // Out of bounds
	}
	c[y] |= (1 << uint(x))
}

// get returns true if a cell is 'on' (revealed).
func (c *ChunkBits) get(x, y int) bool {
	if x < 0 || x >= ChunkSize || y < 0 || y >= ChunkSize {
		return false // Out of bounds
	}
	return (c[y] & (1 << uint(x))) != 0
}

// toBytes converts the ChunkBits bitset to a raw byte slice.
func (c *ChunkBits) toBytes() []byte {
	buf := new(bytes.Buffer)
	buf.Grow(RawChunkBytes)
	for _, val := range c {
		binary.Write(buf, binary.LittleEndian, val)
	}
	return buf.Bytes()
}

// fromBytes populates a ChunkBits from a raw byte slice.
func fromBytes(data []byte) (*ChunkBits, error) {
	if len(data) != RawChunkBytes {
		return nil, fmt.Errorf("invalid data size: got %d bytes, want %d", len(data), RawChunkBytes)
	}
	var chunk ChunkBits
	reader := bytes.NewReader(data)
	for i := 0; i < ChunkSize; i++ {
		if err := binary.Read(reader, binary.LittleEndian, &chunk[i]); err != nil {
			return nil, fmt.Errorf("failed to read chunk data: %w", err)
		}
	}
	return &chunk, nil
}

// CHUNK GENERATORS

func newEmptyChunk() *ChunkBits {
	return &ChunkBits{}
}

func newFullChunk() *ChunkBits {
	var chunk ChunkBits
	for i := range chunk {
		chunk[i] = math.MaxUint64 // Set all 64 bits to 1
	}
	return &chunk
}

func newRandomChunk(density float64, seed int64) *ChunkBits {
	var chunk ChunkBits
	r := rand.New(rand.NewSource(seed))
	for y := 0; y < ChunkSize; y++ {
		for x := 0; x < ChunkSize; x++ {
			if r.Float64() < density {
				chunk.set(x, y)
			}
		}
	}
	return &chunk
}

func newCheckerboardChunk() *ChunkBits {
	var chunk ChunkBits
	for y := 0; y < ChunkSize; y++ {
		for x := 0; x < ChunkSize; x++ {
			if (x+y)%2 == 0 {
				chunk.set(x, y)
			}
		}
	}
	return &chunk
}

func newCircleChunk(cx, cy, radius int) *ChunkBits {
	var chunk ChunkBits
	for y := 0; y < ChunkSize; y++ {
		for x := 0; x < ChunkSize; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			if dx*dx+dy*dy < float64(radius*radius) {
				chunk.set(x, y)
			}
		}
	}
	return &chunk
}

func newHorizontalLinesChunk() *ChunkBits {
	var chunk ChunkBits
	for y := 0; y < ChunkSize; y += 4 {
		for x := 0; x < ChunkSize; x++ {
			chunk.set(x, y)
		}
	}
	return &chunk
}

// ENCODING & DECODING ALGORITHMS

func encodeRaw(chunk *ChunkBits) []byte {
	return chunk.toBytes()
}

func decodeRaw(data []byte) (*ChunkBits, error) {
	return fromBytes(data)
}

func encodeCoordinateList(chunk *ChunkBits) []byte {
	var coords []uint16
	for y := 0; y < ChunkSize; y++ {
		for x := 0; x < ChunkSize; x++ {
			if chunk.get(x, y) {
				coords = append(coords, (uint16(y)<<8)|uint16(x))
			}
		}
	}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, coords)
	return buf.Bytes()
}

func decodeCoordinateList(data []byte) (*ChunkBits, error) {
	var chunk ChunkBits
	reader := bytes.NewReader(data)
	coords := make([]uint16, len(data)/2)
	if len(data) > 0 {
		if err := binary.Read(reader, binary.LittleEndian, &coords); err != nil {
			return nil, err
		}
	}
	for _, coord := range coords {
		y := int(coord >> 8)
		x := int(coord & 0xFF)
		chunk.set(x, y)
	}
	return &chunk, nil
}

func encodeRLE(chunk *ChunkBits) []byte {
	var out []byte
	var runLength uint16
	currentBit := false

	emit := func(n uint16) {
		// little-endian: low byte then high byte
		out = append(out, byte(n), byte(n>>8))
	}

	for y := 0; y < ChunkSize; y++ {
		for x := 0; x < ChunkSize; x++ {
			bit := chunk.get(x, y)
			if bit == currentBit {
				runLength++
			} else {
				emit(runLength)
				currentBit = bit
				runLength = 1
			}
		}
	}
	emit(runLength)
	return out
}

func decodeRLE(data []byte) (*ChunkBits, error) {
	var chunk ChunkBits
	currentBit := false
	x, y := 0, 0
	// RLE stream is a series of little-endian uint16 run lengths
	for i := 0; i+1 < len(data); i += 2 {
		runLength := int(uint16(data[i]) | uint16(data[i+1])<<8)
		for j := 0; j < runLength; j++ {
			if y >= ChunkSize {
				return &chunk, nil
			}
			if currentBit {
				chunk.set(x, y)
			}
			x++
			if x >= ChunkSize {
				x = 0
				y++
			}
		}
		currentBit = !currentBit
	}
	return &chunk, nil
}

func encodeGzip(chunk *ChunkBits) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(chunk.toBytes())
	if err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeGzip(data []byte) (*ChunkBits, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	rawBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return fromBytes(rawBytes)
}

func encodeLZ4(chunk *ChunkBits) ([]byte, error) {
	src := chunk.toBytes()
	dst := make([]byte, lz4.CompressBlockBound(len(src)))
	n, err := lz4.CompressBlock(src, dst, nil)
	if err != nil {
		return nil, err
	}
	if n == 0 { // Incompressible
		return src, nil
	}
	return dst[:n], nil
}

func decodeLZ4(data []byte) (*ChunkBits, error) {
	dst := make([]byte, RawChunkBytes)
	n, err := lz4.UncompressBlock(data, dst)
	if err != nil {
		// Check if it was stored uncompressed
		if len(data) == RawChunkBytes {
			return fromBytes(data)
		}
		return nil, err
	}
	return fromBytes(dst[:n])
}

type QuadtreeNode struct {
	IsLeaf   bool
	Value    bool
	Children [4]*QuadtreeNode
}

func getRegionValue(chunk *ChunkBits, x, y, size int) (isUniform bool, value bool) {
	initialValue := chunk.get(x, y)
	for i := y; i < y+size; i++ {
		for j := x; j < x+size; j++ {
			if chunk.get(j, i) != initialValue {
				return false, false
			}
		}
	}
	return true, initialValue
}

func buildQuadtree(chunk *ChunkBits, x, y, size int) *QuadtreeNode {
	isUniform, value := getRegionValue(chunk, x, y, size)
	if isUniform {
		return &QuadtreeNode{IsLeaf: true, Value: value}
	}
	node := &QuadtreeNode{IsLeaf: false}
	halfSize := size / 2
	node.Children[0] = buildQuadtree(chunk, x, y, halfSize)
	node.Children[1] = buildQuadtree(chunk, x+halfSize, y, halfSize)
	node.Children[2] = buildQuadtree(chunk, x, y+halfSize, halfSize)
	node.Children[3] = buildQuadtree(chunk, x+halfSize, y+halfSize, halfSize)
	return node
}

func (n *QuadtreeNode) serialize(buf *bytes.Buffer) {
	if n.IsLeaf {
		if n.Value {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	} else {
		buf.WriteByte(2)
		for _, child := range n.Children {
			child.serialize(buf)
		}
	}
}

func encodeQuadtree(chunk *ChunkBits) []byte {
	root := buildQuadtree(chunk, 0, 0, ChunkSize)
	var buf bytes.Buffer
	root.serialize(&buf)
	return buf.Bytes()
}

func deserializeQuadtree(buf *bytes.Reader) (*QuadtreeNode, error) {
	b, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	switch b {
	case 0:
		return &QuadtreeNode{IsLeaf: true, Value: false}, nil
	case 1:
		return &QuadtreeNode{IsLeaf: true, Value: true}, nil
	case 2:
		node := &QuadtreeNode{IsLeaf: false}
		for i := 0; i < 4; i++ {
			node.Children[i], err = deserializeQuadtree(buf)
			if err != nil {
				return nil, err
			}
		}
		return node, nil
	default:
		return nil, fmt.Errorf("invalid quadtree node type: %d", b)
	}
}

func (n *QuadtreeNode) reconstruct(chunk *ChunkBits, x, y, size int) {
	if n.IsLeaf {
		if n.Value {
			for i := y; i < y+size; i++ {
				for j := x; j < x+size; j++ {
					chunk.set(j, i)
				}
			}
		}
	} else {
		halfSize := size / 2
		n.Children[0].reconstruct(chunk, x, y, halfSize)
		n.Children[1].reconstruct(chunk, x+halfSize, y, halfSize)
		n.Children[2].reconstruct(chunk, x, y+halfSize, halfSize)
		n.Children[3].reconstruct(chunk, x+halfSize, y+halfSize, halfSize)
	}
}

func decodeQuadtree(data []byte) (*ChunkBits, error) {
	buf := bytes.NewReader(data)
	root, err := deserializeQuadtree(buf)
	if err != nil {
		return nil, err
	}
	var chunk ChunkBits
	root.reconstruct(&chunk, 0, 0, ChunkSize)
	return &chunk, nil
}

type huffmanNode struct {
	char  byte
	freq  int
	left  *huffmanNode
	right *huffmanNode
}

func (n *huffmanNode) IsLeaf() bool {
	return n.left == nil && n.right == nil
}

type huffmanNodeList []*huffmanNode

func (h huffmanNodeList) Len() int           { return len(h) }
func (h huffmanNodeList) Less(i, j int) bool { return h[i].freq < h[j].freq }
func (h huffmanNodeList) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func buildHuffmanTree(freqs map[byte]int) *huffmanNode {
	var nodes huffmanNodeList
	for char, freq := range freqs {
		nodes = append(nodes, &huffmanNode{char: char, freq: freq})
	}
	if len(nodes) == 0 {
		return nil
	}
	for len(nodes) > 1 {
		sort.Sort(nodes)
		left := nodes[0]
		right := nodes[1]
		nodes = nodes[2:]
		newNode := &huffmanNode{
			freq:  left.freq + right.freq,
			left:  left,
			right: right,
		}
		nodes = append(nodes, newNode)
	}
	return nodes[0]
}

func buildHuffmanCodes(node *huffmanNode, prefix string, codes map[byte]string) {
	if node == nil {
		return
	}
	if node.IsLeaf() {
		if prefix == "" {
			codes[node.char] = "0"
		} else {
			codes[node.char] = prefix
		}
		return
	}
	buildHuffmanCodes(node.left, prefix+"0", codes)
	buildHuffmanCodes(node.right, prefix+"1", codes)
}
func encodeRLEHuffman(chunk *ChunkBits) []byte {
	rleData := encodeRLE(chunk)
	if len(rleData) == 0 {
		return nil
	}
	freqs := make(map[byte]int)
	for _, b := range rleData {
		freqs[b]++
	}
	huffmanTree := buildHuffmanTree(freqs)
	codes := make(map[byte]string)
	buildHuffmanCodes(huffmanTree, "", codes)

	var out bytes.Buffer
	out.WriteByte(byte(len(codes)))
	for char, code := range codes {
		out.WriteByte(char)
		out.WriteByte(byte(len(code)))
		out.WriteString(code)
	}

	var encodedBits string
	for _, b := range rleData {
		encodedBits += codes[b]
	}

	padding := (8 - (len(encodedBits) % 8)) % 8
	out.WriteByte(byte(padding))
	encodedBits += string(bytes.Repeat([]byte{'0'}, padding))

	for i := 0; i < len(encodedBits); i += 8 {
		byteStr := encodedBits[i : i+8]
		b, _ := strconv.ParseUint(byteStr, 2, 8)
		out.WriteByte(byte(b))
	}
	return out.Bytes()
}

func decodeRLEHuffman(data []byte) (*ChunkBits, error) {
	if len(data) == 0 {
		return newEmptyChunk(), nil
	}
	reader := bytes.NewReader(data)
	numCodes, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	codes := make(map[string]byte)
	for i := 0; i < int(numCodes); i++ {
		char, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		codeLen, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		codeBytes := make([]byte, codeLen)
		_, err = reader.Read(codeBytes)
		if err != nil {
			return nil, err
		}
		codes[string(codeBytes)] = char
	}

	padding, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	var encodedBits string
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		encodedBits += fmt.Sprintf("%08b", b)
	}

	if len(encodedBits) > int(padding) {
		encodedBits = encodedBits[:len(encodedBits)-int(padding)]
	} else {
		encodedBits = ""
	}

	var rleData []byte
	currentCode := ""
	for _, bit := range encodedBits {
		currentCode += string(bit)
		if char, ok := codes[currentCode]; ok {
			rleData = append(rleData, char)
			currentCode = ""
		}
	}
	return decodeRLE(rleData)
}

// VERIFICATION TESTS

func TestMain(m *testing.M) {
	// Run heavy analysis/benchmarks only when explicitly enabled.
	// Enable with: RUN_COMPRESSION_BENCH=1 go test ./...
	if os.Getenv("RUN_COMPRESSION_BENCH") == "1" {
		runBenchmarks()
	}
	os.Exit(m.Run())
}

func TestEncodingVerification(t *testing.T) {
	testCases := map[string]*ChunkBits{
		"Empty":            newEmptyChunk(),
		"Full":             newFullChunk(),
		"Checkerboard":     newCheckerboardChunk(),
		"Random 10%":       newRandomChunk(0.1, 1),
		"Random 50%":       newRandomChunk(0.5, 1),
		"Random 90%":       newRandomChunk(0.9, 1),
		"Small Circle":     newCircleChunk(32, 32, 8),
		"Large Circle":     newCircleChunk(32, 32, 28),
		"Horizontal Lines": newHorizontalLinesChunk(),
	}

	algorithms := map[string]struct {
		encode func(*ChunkBits) ([]byte, error)
		decode func([]byte) (*ChunkBits, error)
	}{
		"Raw":        {func(c *ChunkBits) ([]byte, error) { return encodeRaw(c), nil }, decodeRaw},
		"CoordList":  {func(c *ChunkBits) ([]byte, error) { return encodeCoordinateList(c), nil }, decodeCoordinateList},
		"RLE":        {func(c *ChunkBits) ([]byte, error) { return encodeRLE(c), nil }, decodeRLE},
		"Gzip":       {encodeGzip, decodeGzip},
		"Quadtree":   {func(c *ChunkBits) ([]byte, error) { return encodeQuadtree(c), nil }, decodeQuadtree},
		"RLEHuffman": {func(c *ChunkBits) ([]byte, error) { return encodeRLEHuffman(c), nil }, decodeRLEHuffman},
		"LZ4":        {encodeLZ4, decodeLZ4},
	}

	for caseName, chunk := range testCases {
		for algName, alg := range algorithms {
			t.Run(fmt.Sprintf("%s-%s", caseName, algName), func(t *testing.T) {
				encoded, err := alg.encode(chunk)
				if err != nil {
					t.Fatalf("encode failed: %v", err)
				}
				decoded, err := alg.decode(encoded)
				if err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if *chunk != *decoded {
					t.Fatalf("decoded chunk does not match original")
				}
			})
		}
	}
}

func runBenchmarks() {
	fmt.Println("--- Chunk Compression Analysis ---")
	fmt.Printf("Analyzing compression for %dx%d chunks (%d bytes raw).\n\n", ChunkSize, ChunkSize, RawChunkBytes)

	chunks := map[string]*ChunkBits{
		"Empty":            newEmptyChunk(),
		"Full":             newFullChunk(),
		"Sparse (1% On)":   newRandomChunk(0.01, 123),
		"Random (10% On)":  newRandomChunk(0.10, 456),
		"Random (50% On)":  newRandomChunk(0.50, 789),
		"Dense (90% On)":   newRandomChunk(0.90, 101),
		"Circle (r=8)":     newCircleChunk(32, 32, 8),
		"Circle (r=28)":    newCircleChunk(32, 32, 28),
		"Horizontal Lines": newHorizontalLinesChunk(),
	}

	results := make(map[string]map[string]int)

	for name, chunk := range chunks {
		results[name] = make(map[string]int)
		results[name]["Raw"] = len(encodeRaw(chunk))
		results[name]["Coord List"] = len(encodeCoordinateList(chunk))
		results[name]["RLE"] = len(encodeRLE(chunk))
		gzipped, _ := encodeGzip(chunk)
		results[name]["Gzip"] = len(gzipped)
		lz4ed, _ := encodeLZ4(chunk)
		results[name]["LZ4"] = len(lz4ed)
		results[name]["Quadtree"] = len(encodeQuadtree(chunk))
		results[name]["RLE+Huffman"] = len(encodeRLEHuffman(chunk))
	}

	fmt.Println("--- Compressed Size Comparison (in bytes) ---")
	fmt.Printf("%-18s | %-12s | %-12s | %-12s | %-12s | %-12s | %-12s | %-12s\n", "Chunk Type", "Raw", "Coord List", "RLE", "Gzip", "Quadtree", "RLE+Huffman", "LZ4")
	fmt.Println("---------------------------------------------------------------------------------------------------------------------------")

	// Sort for consistent output
	var names []string
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		sizes := results[name]
		fmt.Printf("%-18s | %-12d | %-12d | %-12d | %-12d | %-12d | %-12d | %-12d\n",
			name,
			sizes["Raw"],
			sizes["Coord List"],
			sizes["RLE"],
			sizes["Gzip"],
			sizes["Quadtree"],
			sizes["RLE+Huffman"],
			sizes["LZ4"],
		)
	}
	fmt.Println("")
	fmt.Println("--- Performance Benchmarks (ns/op) ---")
	fmt.Println("Running performance tests... (this may take a moment)")
	fmt.Println("Performance benchmark results are integrated into the `go test` output.")

	testing.Benchmark(func(b *testing.B) {
		for name, chunk := range chunks {
			b.Run(fmt.Sprintf("Encode/Raw/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeRaw(chunk)
				}
			})
			b.Run(fmt.Sprintf("Encode/CoordList/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeCoordinateList(chunk)
				}
			})
			b.Run(fmt.Sprintf("Encode/RLE/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeRLE(chunk)
				}
			})
			b.Run(fmt.Sprintf("Encode/Gzip/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeGzip(chunk)
				}
			})
			b.Run(fmt.Sprintf("Encode/Quadtree/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeQuadtree(chunk)
				}
			})
			b.Run(fmt.Sprintf("Encode/RLEHuffman/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeRLEHuffman(chunk)
				}
			})
			b.Run(fmt.Sprintf("Encode/LZ4/%s", name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					encodeLZ4(chunk)
				}
			})
		}
	})
}
