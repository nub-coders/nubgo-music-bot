package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	client        *mongo.Client
	access        *mongo.Collection
	botSettings   *mongo.Collection
	chatPlayback  *mongo.Collection
	userPlaylists *mongo.Collection
}

func OpenMongo(ctx context.Context, uri, databaseName string) (*Mongo, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect MongoDB: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}
	database := client.Database(databaseName)
	store := &Mongo{
		client:        client,
		access:        database.Collection("user_sessions"),
		botSettings:   database.Collection("collection"),
		chatPlayback:  database.Collection("chat_playback"),
		userPlaylists: database.Collection("user_playlists"),
	}
	if err := store.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return store, nil
}

func (m *Mongo) ensureIndexes(ctx context.Context) error {
	if _, err := m.access.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "bot_id", Value: 1}}}); err != nil {
		return fmt.Errorf("create user_sessions index: %w", err)
	}
	if _, err := m.botSettings.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "bot_id", Value: 1}}}); err != nil {
		return fmt.Errorf("create collection index: %w", err)
	}
	if _, err := m.chatPlayback.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "chat_id", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
		return fmt.Errorf("create chat_playback index: %w", err)
	}
	if _, err := m.userPlaylists.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
		return fmt.Errorf("create user_playlists index: %w", err)
	}
	return nil
}

func (m *Mongo) IsSudo(ctx context.Context, botID, userID int64) (bool, error) {
	return contains(ctx, m.access, bson.M{"bot_id": botID, "SUDOERS": userID})
}
func (m *Mongo) IsBotAdmin(ctx context.Context, botID, userID int64) (bool, error) {
	return contains(ctx, m.botSettings, bson.M{"bot_id": botID, "admins": userID})
}
func (m *Mongo) IsAuthorized(ctx context.Context, botID, chatID, userID int64) (bool, error) {
	return contains(ctx, m.access, bson.M{"bot_id": botID, "auth_users." + strconv.FormatInt(chatID, 10): userID})
}
func (m *Mongo) IsBlocked(ctx context.Context, botID, userID int64) (bool, error) {
	return contains(ctx, m.botSettings, bson.M{"bot_id": botID, "busers": userID})
}
func contains(ctx context.Context, collection *mongo.Collection, filter bson.M) (bool, error) {
	err := collection.FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return err == nil, err
}

func (m *Mongo) AddAuthorized(ctx context.Context, botID, chatID, userID int64) error {
	field := "auth_users." + strconv.FormatInt(chatID, 10)
	_, err := m.access.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$addToSet": bson.M{field: userID}}, options.Update().SetUpsert(true))
	return err
}
func (m *Mongo) RemoveAuthorized(ctx context.Context, botID, chatID, userID int64) error {
	field := "auth_users." + strconv.FormatInt(chatID, 10)
	_, err := m.access.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$pull": bson.M{field: userID}})
	return err
}
func (m *Mongo) AddSudo(ctx context.Context, botID, userID int64) error {
	_, err := m.access.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$addToSet": bson.M{"SUDOERS": userID}}, options.Update().SetUpsert(true))
	return err
}
func (m *Mongo) RemoveSudo(ctx context.Context, botID, userID int64) error {
	_, err := m.access.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$pull": bson.M{"SUDOERS": userID}})
	return err
}
func (m *Mongo) ListSudo(ctx context.Context, botID int64) ([]int64, error) {
	return arrayField(ctx, m.access, bson.M{"bot_id": botID}, "SUDOERS")
}
func (m *Mongo) ListAuthorized(ctx context.Context, botID, chatID int64) ([]int64, error) {
	field := "auth_users." + strconv.FormatInt(chatID, 10)
	return arrayField(ctx, m.access, bson.M{"bot_id": botID}, field)
}
func (m *Mongo) AddBlocked(ctx context.Context, botID, userID int64) error {
	_, err := m.botSettings.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$addToSet": bson.M{"busers": userID}}, options.Update().SetUpsert(true))
	return err
}
func (m *Mongo) RemoveBlocked(ctx context.Context, botID, userID int64) error {
	_, err := m.botSettings.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$pull": bson.M{"busers": userID}})
	return err
}
func (m *Mongo) ListBlocked(ctx context.Context, botID int64) ([]int64, error) {
	return arrayField(ctx, m.botSettings, bson.M{"bot_id": botID}, "busers")
}
func (m *Mongo) SeedAdmins(ctx context.Context, botID int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := m.botSettings.UpdateOne(ctx, bson.M{"bot_id": botID}, bson.M{"$addToSet": bson.M{"admins": bson.M{"$each": userIDs}}}, options.Update().SetUpsert(true))
	return err
}
func (m *Mongo) RecordPlay(ctx context.Context, chatID int64) error {
	now := time.Now().Unix()
	_, err := m.chatPlayback.UpdateOne(ctx, bson.M{"chat_id": chatID}, bson.M{
		"$set":  bson.M{"last_played": now},
		"$inc":  bson.M{"play_count": 1},
		"$push": bson.M{"play_dates": bson.M{"$each": []int64{now}, "$slice": -5000}},
	}, options.Update().SetUpsert(true))
	return err
}
func (m *Mongo) TopChats(ctx context.Context, limit int) ([]ChatStat, error) {
	if limit <= 0 {
		limit = 10
	}
	cursor, err := m.chatPlayback.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"chat_id": 1, "play_count": 1, "last_played": 1,
	}).SetSort(bson.M{"play_count": -1}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var stats []ChatStat
	for cursor.Next(ctx) {
		var raw struct {
			ChatID     int64 `bson:"chat_id"`
			PlayCount  int64 `bson:"play_count"`
			LastPlayed int64 `bson:"last_played"`
		}
		if err := cursor.Decode(&raw); err != nil {
			continue
		}
		stats = append(stats, ChatStat{ChatID: raw.ChatID, PlayCount: raw.PlayCount, LastPlayed: raw.LastPlayed})
	}
	return stats, cursor.Err()
}
func (m *Mongo) SetWelcome(ctx context.Context, chatID int64, text string) error {
	_, err := m.chatPlayback.UpdateOne(ctx, bson.M{"chat_id": chatID}, bson.M{"$set": bson.M{"welcome": text}}, options.Update().SetUpsert(true))
	return err
}
func (m *Mongo) GetWelcome(ctx context.Context, chatID int64) (string, error) {
	var raw struct {
		Welcome string `bson:"welcome"`
	}
	err := m.chatPlayback.FindOne(ctx, bson.M{"chat_id": chatID}).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return raw.Welcome, nil
}

func (m *Mongo) CreatePlaylist(ctx context.Context, userID int64, name string) (Playlist, error) {
	name = cleanPlaylistName(name)
	playlist := Playlist{ID: newPlaylistID(), Name: name, Tracks: []PlaylistTrack{}}
	_, err := m.userPlaylists.UpdateOne(ctx, bson.M{"user_id": userID},
		bson.M{"$push": bson.M{"playlists": playlist}}, options.Update().SetUpsert(true))
	if err != nil {
		return Playlist{}, err
	}
	return playlist, nil
}
func (m *Mongo) GetPlaylists(ctx context.Context, userID int64) ([]Playlist, error) {
	var raw struct {
		Playlists []Playlist `bson:"playlists"`
	}
	err := m.userPlaylists.FindOne(ctx, bson.M{"user_id": userID}).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return raw.Playlists, nil
}
func (m *Mongo) GetPlaylistByName(ctx context.Context, userID int64, name string) (*Playlist, error) {
	playlists, err := m.GetPlaylists(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	for index := range playlists {
		if strings.ToLower(playlists[index].Name) == normalized {
			copy := playlists[index]
			return &copy, nil
		}
	}
	return nil, nil
}
func (m *Mongo) RenamePlaylist(ctx context.Context, userID int64, playlistID, name string) error {
	_, err := m.userPlaylists.UpdateOne(ctx, bson.M{"user_id": userID, "playlists.id": playlistID},
		bson.M{"$set": bson.M{"playlists.$.name": cleanPlaylistName(name)}})
	return err
}
func (m *Mongo) DeletePlaylist(ctx context.Context, userID int64, playlistID string) error {
	_, err := m.userPlaylists.UpdateOne(ctx, bson.M{"user_id": userID},
		bson.M{"$pull": bson.M{"playlists": bson.M{"id": playlistID}}})
	return err
}
func (m *Mongo) AddPlaylistTrack(ctx context.Context, userID int64, playlistID string, track PlaylistTrack) error {
	_, err := m.userPlaylists.UpdateOne(ctx, bson.M{"user_id": userID, "playlists.id": playlistID},
		bson.M{"$push": bson.M{"playlists.$.tracks": track}})
	return err
}
func (m *Mongo) RemovePlaylistTrack(ctx context.Context, userID int64, playlistID string, index int) error {
	if index < 0 {
		return nil
	}
	// A pull-by-index needs the array slice server-side; fetch and rewrite is
	// simpler and safe because the per-user document is small (<=50 tracks).
	var raw struct {
		Playlists []Playlist `bson:"playlists"`
	}
	err := m.userPlaylists.FindOne(ctx, bson.M{"user_id": userID}).Decode(&raw)
	if err != nil {
		return err
	}
	for i := range raw.Playlists {
		if raw.Playlists[i].ID != playlistID {
			continue
		}
		if index >= len(raw.Playlists[i].Tracks) {
			return nil
		}
		raw.Playlists[i].Tracks = append(raw.Playlists[i].Tracks[:index], raw.Playlists[i].Tracks[index+1:]...)
		_, err := m.userPlaylists.UpdateOne(ctx, bson.M{"user_id": userID, "playlists.id": playlistID},
			bson.M{"$set": bson.M{"playlists.$.tracks": raw.Playlists[i].Tracks}})
		return err
	}
	return nil
}
func (m *Mongo) Close(ctx context.Context) error { return m.client.Disconnect(ctx) }

func (m *Mongo) ChatIDs(ctx context.Context) ([]int64, error) {
	cursor, err := m.chatPlayback.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"chat_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var ids []int64
	for cursor.Next(ctx) {
		var raw struct {
			ChatID int64 `bson:"chat_id"`
		}
		if err := cursor.Decode(&raw); err == nil && raw.ChatID != 0 {
			ids = append(ids, raw.ChatID)
		}
	}
	return ids, cursor.Err()
}

func cleanPlaylistName(name string) string {
	return strings.TrimSpace(name)
}
func newPlaylistID() string {
	var buffer [6]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer[:])
}

// arrayField reads a stored numeric-array field from the first matching doc.
// Dotted paths (e.g. "auth_users.-100123") are supported by navigating the
// decoded document rather than using an aggregation projection.
func arrayField(ctx context.Context, collection *mongo.Collection, filter bson.M, field string) ([]int64, error) {
	var raw bson.M
	err := collection.FindOne(ctx, filter).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	parts := strings.Split(field, ".")
	var current any = raw
	for _, part := range parts {
		doc, ok := current.(bson.M)
		if !ok {
			return nil, nil
		}
		current = doc[part]
	}
	switch values := current.(type) {
	case []int64:
		return values, nil
	case bson.A:
		result := make([]int64, 0, len(values))
		for _, value := range values {
			if number, ok := value.(int64); ok {
				result = append(result, number)
			}
		}
		return result, nil
	case nil:
		return nil, nil
	default:
		return nil, nil
	}
}
