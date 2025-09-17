package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type IconRecord struct {
	Domain      string
	IconURL     string
	SourceURL   string
	ETag        *string
	Width       *int32
	Height      *int32
	ContentType *string
	UpdatedAt   time.Time
}

type DB struct{ Pool *pgxpool.Pool }

func New(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil { return nil, err }
	if err := pool.Ping(ctx); err != nil { return nil, err }
	return &DB{Pool: pool}, nil
}

func (d *DB) FindByDomain(ctx context.Context, domain string) (*IconRecord, error) {
	row := d.Pool.QueryRow(ctx, `
		SELECT domain, icon_url, source_url, etag, width, height, content_type, updated_at
		FROM icons WHERE domain=$1`, domain)
	rec := IconRecord{}
	if err := row.Scan(&rec.Domain, &rec.IconURL, &rec.SourceURL, &rec.ETag, &rec.Width, &rec.Height, &rec.ContentType, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (d *DB) Upsert(ctx context.Context, rec IconRecord) error {
	_, err := d.Pool.Exec(ctx, `
		INSERT INTO icons (domain, icon_url, source_url, etag, width, height, content_type, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7, NOW())
		ON CONFLICT (domain) DO UPDATE SET
		  icon_url=EXCLUDED.icon_url,
		  source_url=EXCLUDED.source_url,
		  etag=EXCLUDED.etag,
		  width=EXCLUDED.width,
		  height=EXCLUDED.height,
		  content_type=EXCLUDED.content_type,
		  updated_at=NOW();
	`, rec.Domain, rec.IconURL, rec.SourceURL, rec.ETag, rec.Width, rec.Height, rec.ContentType)
	return err
}
