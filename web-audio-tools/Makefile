ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)
VENV := ai-service/.venv

.PHONY: install install-ml wasm frontend backend ai dev-frontend test

install:
	cd frontend && npm install
	uv venv -q $(VENV) --python 3.12 && uv pip install -q -p $(VENV) -r ai-service/requirements-dev.txt

# Optional heavy models (DeepFilterNet, Demucs, faster-whisper): several GB.
install-ml:
	uv pip install -p $(VENV) -r ai-service/requirements-ml.txt

wasm:
	cd frontend && npm run build:wasm

frontend:
	cd frontend && npm run build

# Serves the API and the built frontend on :8080.
backend:
	cd $(ROOT) && go run ./web-audio-tools/backend

ai:
	cd ai-service && .venv/bin/python app.py

dev-frontend:
	cd frontend && npm run dev

test:
	cd dsp-wasm && cargo test --release
	cd $(ROOT) && go vet ./... && go test -race ./...
	cd ai-service && .venv/bin/python -m pytest -q
	cd frontend && npm run check
