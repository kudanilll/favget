run:
	APP_ENV=dev PORT=8080 \
	DATABASE_URL=$(DATABASE_URL) \
	REDIS_URL=$(REDIS_URL) \
	CLOUDINARY_URL=$(CLOUDINARY_URL) \
	go run ./cmd/server

docker-build:
	docker build -t favget:latest .

docker-run:
	docker run --rm -p 8080:8080 \
	-e PORT=8080 \
	-e DATABASE_URL="$(DATABASE_URL)" \
	-e REDIS_URL="$(REDIS_URL)" \
	-e CLOUDINARY_URL="$(CLOUDINARY_URL)" \
	favget:latest
