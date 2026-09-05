package main

import (
	"slices"
	"time"

	pb "github.com/henri123lemoine/infiniteminesweeper.com/backend/gen/proto"
)

var overviewGlobalLODs = [...]uint32{1, 2, 4, 8}

const maxOverviewRegionPixels = 8 << 20
const overviewGlobalPadding = 8
const overviewDetailCacheMaxEntries = 1024

type overviewTile struct {
	LOD1  [1]byte
	LOD2  [4]byte
	LOD4  [16]byte
	LOD8  [64]byte
	LOD12 [144]byte
	LOD16 [256]byte
}

func (t *overviewTile) pixels(lod uint32) []byte {
	if t == nil {
		return nil
	}
	switch lod {
	case 1:
		return t.LOD1[:]
	case 2:
		return t.LOD2[:]
	case 4:
		return t.LOD4[:]
	case 8:
		return t.LOD8[:]
	case 12:
		return t.LOD12[:]
	case 16:
		return t.LOD16[:]
	default:
		return nil
	}
}

type overviewDetail struct {
	cid         ChunkID
	full, lod32 []byte
}

// Keep the zoom-out runway warm; bound the two largest, closest zoom levels.
// Caller must hold stateMu for writing.
func (s *Server) overviewPixelsLocked(cid ChunkID, lod uint32) []byte {
	if lod <= 16 {
		return s.overviewTiles[cid].pixels(lod)
	}
	element := s.overviewDetails[cid]
	if element == nil {
		full := s.computeFullTileData(cid)
		if full == nil {
			return nil
		}
		if len(s.overviewDetails) >= overviewDetailCacheMaxEntries {
			oldest := s.overviewDetailLRU.Back()
			delete(s.overviewDetails, oldest.Value.(*overviewDetail).cid)
			s.overviewDetailLRU.Remove(oldest)
		}
		element = s.overviewDetailLRU.PushFront(&overviewDetail{cid: cid, full: full})
		s.overviewDetails[cid] = element
	} else {
		s.overviewDetailLRU.MoveToFront(element)
	}
	detail := element.Value.(*overviewDetail)
	if lod == 64 {
		return detail.full
	}
	if lod != 32 {
		return nil
	}
	if detail.lod32 == nil {
		detail.lod32 = downsampleOverviewColor(detail.full, 32)
	}
	return detail.lod32
}

func (s *Server) invalidateOverviewDetailLocked(cid ChunkID) {
	if element := s.overviewDetails[cid]; element != nil {
		s.overviewDetailLRU.Remove(element)
		delete(s.overviewDetails, cid)
	}
}

type overviewImage struct {
	OriginX, OriginY          int64
	WidthChunks, HeightChunks int
	Revision                  uint64
	Pixels                    []byte
	Encoded                   []byte
}

type overviewSubscription struct {
	Global                    bool
	OriginX, OriginY          int64
	WidthChunks, HeightChunks int
	Ready                     bool
	Token                     uint64
	Pending                   map[ChunkID]struct{}
}

type overviewPendingSend struct {
	players []*Player
	msg     *pb.Msg
}

func validOverviewLOD(lod uint32) bool {
	return lod == 1 || lod == 2 || lod == 4 || lod == 8 || lod == 12 || lod == 16 || lod == 32 || lod == 64
}

func (s *Server) computeOverviewTile(cid ChunkID) *overviewTile {
	full := s.computeFullTileData(cid)
	if full == nil {
		return nil
	}
	return computeOverviewTileFromFull(full)
}

func computeOverviewTileFromFull(full []byte) *overviewTile {
	tile := &overviewTile{}
	copy(tile.LOD1[:], downsampleOverviewColor(full, 1))
	copy(tile.LOD2[:], downsampleOverviewColor(full, 2))
	copy(tile.LOD4[:], downsampleOverviewColor(full, 4))
	copy(tile.LOD8[:], downsampleOverviewColor(full, 8))
	copy(tile.LOD12[:], downsampleOverviewColor(full, 12))
	copy(tile.LOD16[:], downsampleOverviewColor(full, 16))
	return tile
}

func blitOverviewTile(dst []byte, dstWidth int, chunkX, chunkY int, tile []byte, lod int) {
	if len(tile) != lod*lod {
		return
	}
	for row := 0; row < lod; row++ {
		dstStart := (chunkY*lod+row)*dstWidth + chunkX*lod
		copy(dst[dstStart:dstStart+lod], tile[row*lod:(row+1)*lod])
	}
}

func (s *Server) overviewBoundsLocked() (minX, minY, maxX, maxY int64, ok bool) {
	for cid := range s.overviewTiles {
		if !ok {
			minX, maxX, minY, maxY, ok = cid.X, cid.X, cid.Y, cid.Y, true
			continue
		}
		if cid.X < minX {
			minX = cid.X
		}
		if cid.X > maxX {
			maxX = cid.X
		}
		if cid.Y < minY {
			minY = cid.Y
		}
		if cid.Y > maxY {
			maxY = cid.Y
		}
	}
	return
}

func (s *Server) rebuildOverviewImagesLocked() {
	minX, minY, maxX, maxY, ok := s.overviewBoundsLocked()
	if !ok {
		minX, minY, maxX, maxY = 0, 0, 0, 0
	}
	minX -= overviewGlobalPadding
	minY -= overviewGlobalPadding
	maxX += overviewGlobalPadding
	maxY += overviewGlobalPadding
	widthChunks := int(maxX-minX) + 1
	heightChunks := int(maxY-minY) + 1
	images := make(map[uint32]*overviewImage, len(overviewGlobalLODs))
	for _, lod := range overviewGlobalLODs {
		pixelCount := widthChunks * heightChunks * int(lod*lod)
		if pixelCount > maxOverviewRegionPixels {
			continue
		}
		images[lod] = &overviewImage{
			OriginX: minX, OriginY: minY,
			WidthChunks: widthChunks, HeightChunks: heightChunks,
			Revision: s.overviewRevision,
			Pixels:   make([]byte, pixelCount),
		}
	}
	for cid, tile := range s.overviewTiles {
		for lod, image := range images {
			blitOverviewTile(
				image.Pixels,
				image.WidthChunks*int(lod),
				int(cid.X-image.OriginX),
				int(cid.Y-image.OriginY),
				tile.pixels(lod),
				int(lod),
			)
		}
	}
	s.overviewImages = images
}

func (s *Server) rebuildOverviewLocked() {
	clear(s.overviewDetails)
	s.overviewDetailLRU.Init()
	tiles := make(map[ChunkID]*overviewTile, len(s.chunks)+len(s.flags))
	minimapCache16 := make(map[ChunkID][]byte, len(s.chunks)+len(s.flags))
	for cid := range s.chunks {
		full := s.computeFullTileData(cid)
		tiles[cid] = computeOverviewTileFromFull(full)
		minimapCache16[cid] = downsampleTo(cid, full, 16)
	}
	for cid := range s.flags {
		if _, exists := tiles[cid]; exists {
			continue
		}
		full := s.computeFullTileData(cid)
		tiles[cid] = computeOverviewTileFromFull(full)
		minimapCache16[cid] = downsampleTo(cid, full, 16)
	}
	s.overviewTiles = tiles
	s.minimapCache16 = minimapCache16
	s.overviewRevision++
	s.rebuildOverviewImagesLocked()
	s.primeGlobalOverviewLocked(8)
}

func overviewSnapshotMessage(lod uint32, image *overviewImage, pixels []byte, unchanged bool) *pb.Msg {
	return &pb.Msg{Payload: &pb.Msg_OverviewSnapshot{OverviewSnapshot: &pb.OverviewSnapshot{
		Lod: lod, OriginX: int32(image.OriginX), OriginY: int32(image.OriginY),
		WidthChunks: uint32(image.WidthChunks), HeightChunks: uint32(image.HeightChunks),
		Revision: image.Revision, Pixels: pixels, Global: true, Unchanged: unchanged,
	}}}
}

func (s *Server) primeGlobalOverviewLocked(lod uint32) {
	image := s.overviewImages[lod]
	if image == nil {
		return
	}
	image.Encoded = mustProto(overviewSnapshotMessage(lod, image, image.Pixels, false))
}

func (s *Server) refreshOverviewDirtyLocked() ([]ChunkID, uint64) {
	if len(s.overviewDirty) == 0 {
		return nil, s.overviewRevision
	}
	dirty := make([]ChunkID, 0, len(s.overviewDirty))
	for cid := range s.overviewDirty {
		dirty = append(dirty, cid)
	}
	clear(s.overviewDirty)
	s.overviewRevision++
	rebuildImages := false
	for _, cid := range dirty {
		s.invalidateOverviewDetailLocked(cid)
		tile := s.computeOverviewTile(cid)
		if tile == nil {
			delete(s.overviewTiles, cid)
			rebuildImages = true
			continue
		}
		s.overviewTiles[cid] = tile
		for lod, image := range s.overviewImages {
			x := int(cid.X - image.OriginX)
			y := int(cid.Y - image.OriginY)
			if x < 0 || y < 0 || x >= image.WidthChunks || y >= image.HeightChunks {
				rebuildImages = true
				continue
			}
			blitOverviewTile(image.Pixels, image.WidthChunks*int(lod), x, y, tile.pixels(lod), int(lod))
			image.Revision = s.overviewRevision
			image.Encoded = nil
		}
	}
	if rebuildImages {
		s.rebuildOverviewImagesLocked()
	}
	return dirty, s.overviewRevision
}

func overviewSubContains(sub overviewSubscription, cid ChunkID) bool {
	if sub.Global {
		return true
	}
	return cid.X >= sub.OriginX && cid.Y >= sub.OriginY &&
		cid.X < sub.OriginX+int64(sub.WidthChunks) &&
		cid.Y < sub.OriginY+int64(sub.HeightChunks)
}

func (s *Server) overviewPatchLocked(lod uint32, sub overviewSubscription, dirty []ChunkID, revision uint64) *pb.Msg {
	updates := make([]*pb.OverviewTileUpdate, 0, len(dirty))
	for _, cid := range dirty {
		if !overviewSubContains(sub, cid) {
			continue
		}
		pixels := s.overviewPixelsLocked(cid, lod)
		if pixels == nil {
			pixels = make([]byte, int(lod*lod))
		}
		updates = append(updates, &pb.OverviewTileUpdate{
			Tile: &pb.TileRef{X: int32(cid.X), Y: int32(cid.Y)},
			Data: slices.Clone(pixels),
		})
	}
	if len(updates) == 0 {
		return nil
	}
	for _, update := range updates {
		s.overviewPatchBytes += uint64(len(update.Data))
	}
	return &pb.Msg{Payload: &pb.Msg_OverviewPatch{OverviewPatch: &pb.OverviewPatch{
		Lod: lod, Revision: revision, Tiles: updates,
	}}}
}

type overviewPatchKey struct {
	lod           uint32
	global        bool
	x, y          int64
	width, height int
}

func (s *Server) broadcastOverview() {
	s.stateMu.Lock()
	dirty, revision := s.refreshOverviewDirtyLocked()
	if len(dirty) == 0 {
		s.stateMu.Unlock()
		return
	}
	patches := make(map[overviewPatchKey]*overviewPendingSend)
	for player, byLOD := range s.overviewSubs {
		for lod, sub := range byLOD {
			if !sub.Ready {
				if sub.Pending == nil {
					sub.Pending = make(map[ChunkID]struct{})
				}
				for _, cid := range dirty {
					if overviewSubContains(sub, cid) {
						sub.Pending[cid] = struct{}{}
					}
				}
				byLOD[lod] = sub
				continue
			}
			key := overviewPatchKey{lod: lod, global: sub.Global}
			if !sub.Global {
				key.x, key.y = sub.OriginX, sub.OriginY
				key.width, key.height = sub.WidthChunks, sub.HeightChunks
			}
			patch, exists := patches[key]
			if !exists {
				patch = &overviewPendingSend{msg: s.overviewPatchLocked(lod, sub, dirty, revision)}
				patches[key] = patch
			}
			if patch.msg != nil {
				patch.players = append(patch.players, player)
			}
		}
	}
	for _, patch := range patches {
		if patch.msg != nil {
			for _, tile := range patch.msg.GetOverviewPatch().Tiles {
				s.overviewPatchBytes += uint64(len(tile.Data) * (len(patch.players) - 1))
			}
		}
	}
	s.stateMu.Unlock()
	var wireBytes uint64
	for _, patch := range patches {
		if patch.msg == nil {
			continue
		}
		data := mustProto(patch.msg)
		wireBytes += uint64(len(data) * len(patch.players))
		for _, player := range patch.players {
			s.sendOverview(player, data)
		}
	}
	s.stateMu.Lock()
	s.overviewWireBytes += wireBytes
	s.stateMu.Unlock()
}

func (s *Server) runOverviewBroadcaster() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		s.broadcastOverview()
	}
}

func (s *Server) assembleOverviewRegionLocked(lod uint32, originX, originY int64, widthChunks, heightChunks int) []byte {
	widthPixels := widthChunks * int(lod)
	pixels := make([]byte, widthPixels*heightChunks*int(lod))
	for y := 0; y < heightChunks; y++ {
		for x := 0; x < widthChunks; x++ {
			cid := ChunkID{X: originX + int64(x), Y: originY + int64(y)}
			tilePixels := s.overviewPixelsLocked(cid, lod)
			if tilePixels != nil {
				blitOverviewTile(pixels, widthPixels, x, y, tilePixels, int(lod))
			}
		}
	}
	return pixels
}

func (s *Server) handleOverviewRequest(player *Player, request *pb.OverviewRequest) {
	if request == nil || !validOverviewLOD(request.Lod) {
		return
	}
	lod := request.Lod
	s.stateMu.Lock()
	if request.Subscribe && s.overviewSubs[player] == nil {
		s.overviewSubs[player] = make(map[uint32]overviewSubscription)
	}

	global := request.Global
	originX, originY := int64(request.OriginX), int64(request.OriginY)
	widthChunks, heightChunks := int(request.WidthChunks), int(request.HeightChunks)
	var pixels []byte
	var encoded []byte
	revision := s.overviewRevision
	var globalImage *overviewImage
	if global {
		if image := s.overviewImages[lod]; image != nil {
			globalImage = image
			originX, originY = image.OriginX, image.OriginY
			widthChunks, heightChunks = image.WidthChunks, image.HeightChunks
			revision = image.Revision
			if request.KnownRevision != revision {
				if image.Encoded != nil {
					encoded = image.Encoded
				} else {
					pixels = slices.Clone(image.Pixels)
				}
			}
		} else {
			global = false
		}
	}
	if !global {
		if widthChunks <= 0 || heightChunks <= 0 ||
			widthChunks*heightChunks*int(lod*lod) > maxOverviewRegionPixels {
			s.stateMu.Unlock()
			return
		}
		if request.KnownRevision != revision {
			pixels = s.assembleOverviewRegionLocked(lod, originX, originY, widthChunks, heightChunks)
		}
	}
	sub := overviewSubscription{
		Global: global, OriginX: originX, OriginY: originY,
		WidthChunks: widthChunks, HeightChunks: heightChunks,
		Token: s.overviewRequests + 1,
	}
	if request.Subscribe {
		s.overviewSubs[player][lod] = sub
	}
	s.overviewRequests++
	if globalImage != nil && request.KnownRevision != revision {
		s.overviewSnapBytes += uint64(len(globalImage.Pixels))
	} else {
		s.overviewSnapBytes += uint64(len(pixels))
	}
	msg := &pb.Msg{Payload: &pb.Msg_OverviewSnapshot{OverviewSnapshot: &pb.OverviewSnapshot{
		Lod: lod, OriginX: int32(originX), OriginY: int32(originY),
		WidthChunks: uint32(widthChunks), HeightChunks: uint32(heightChunks),
		Revision: revision, Pixels: pixels, Global: global,
		Unchanged: request.KnownRevision == revision,
	}}}
	s.stateMu.Unlock()

	if encoded == nil {
		encoded = mustProto(msg)
		if globalImage != nil && len(pixels) != 0 {
			s.stateMu.Lock()
			if current := s.overviewImages[lod]; current != nil && current.Revision == revision && current.Encoded == nil {
				current.Encoded = encoded
			}
			s.stateMu.Unlock()
		}
	}
	s.sendOverview(player, encoded)

	s.stateMu.Lock()
	s.overviewWireBytes += uint64(len(encoded))
	var catchup *pb.Msg
	if request.Subscribe {
		byLOD := s.overviewSubs[player]
		if current, exists := byLOD[lod]; exists && current.Token == sub.Token {
			pending := make([]ChunkID, 0, len(current.Pending))
			for cid := range current.Pending {
				pending = append(pending, cid)
			}
			current.Ready = true
			current.Pending = nil
			byLOD[lod] = current
			catchup = s.overviewPatchLocked(lod, current, pending, s.overviewRevision)
		}
	}
	s.stateMu.Unlock()
	if catchup != nil {
		data := mustProto(catchup)
		s.stateMu.Lock()
		s.overviewWireBytes += uint64(len(data))
		s.stateMu.Unlock()
		s.sendOverview(player, data)
	}
}

func (s *Server) sendOverview(player *Player, data []byte) {
	select {
	case <-player.done:
		return
	default:
	}
	select {
	case player.Send <- data:
		player.dropMisses = 0
	default:
		player.dropMisses++
		if player.dropMisses > 32 && player.Conn != nil {
			player.Conn.Close()
		}
	}
}

func (s *Server) releaseOverview(player *Player) {
	s.stateMu.Lock()
	delete(s.overviewSubs, player)
	s.stateMu.Unlock()
}
