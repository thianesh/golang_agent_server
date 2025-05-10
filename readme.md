# Golang Ollama Agent Server with Tools

An end-to-end AI agent framework powered by Go, integrating media processing, Python tooling, and a complete frontend-backend stack.

---

## 🔧 Project Overview

This project aims to build a fully operational agent framework with the following features:

* ✅ **End-to-End Agent Stack**: Combines a modern **frontend** with a powerful **backend** written in Golang.
* 🎞️ **Media Engine**: Handles media input/output, processing, and transformation.
* 🔁 **Python IPC Media Transfer**: Media data is exchanged between Go and Python via **inter-process communication (IPC)**, enabling advanced tooling and AI-based inference pipelines.

---

## 📦 Current Progress

As of **10-May-2025**:

* Backend server setup using **Supabase** is underway.
* IPC bridge to Python-based tools for handling media processing is functioning.
* Project architecture is being organized into modular layers to support agent tools, plugin interfaces, and media pipelines.

---

## 🗂️ Project Structure (Planned)

```
/backend      -> Golang server, Supabase integration
/frontend     -> Vue 3 (planned) or other modern SPA
/media-engine -> Audio/video processing components
/ipc-python   -> Python tools connected via IPC for inference
```

---

## 🚀 Getting Started (Coming Soon)

Instructions to set up and run the project locally will be added after the backend and Supabase integration is complete.

---

## 🔜 Roadmap

* [ ] Finalize Supabase auth and data models
* [ ] Add REST/gRPC API interface
* [ ] Develop Vue 3 frontend with real-time media interaction
* [ ] Extend Python toolset for audio transcription and NLP tasks
* [ ] Add Docker support and deployment setup

---

## 🧪 Tech Stack

* **Backend**: Golang, Supabase, gRPC/REST
* **Frontend**: Vue 3 (Composition API)
* **Media Engine**: Custom-built
* **IPC Tools**: Python (ASR, TTS, etc.)
* **Database**: Supabase (PostgreSQL)

---

## 🧠 Inspiration

This project is designed to build modular, voice/media-enabled agents with real-world deployment goals, inspired by the growing intersection of LLMs and voice AI.
