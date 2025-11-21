# Matrix Gemini Bot

## 🛠️ Prerequisites

- **Go** installed.
- **libolm** (Required for E2EE support):
- A **Matrix Account** for the bot.
- A **Google Gemini API Key** (Get one at [aistudio.google.com](https://aistudio.google.com)).

## 🚀 Installation & Setup

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/rjanupam/matrixGemini.git
    cd matrixGemini
    ```

2.  **Install Go dependencies:**

    ```bash
    go mod tidy
    ```

3.  **Configure the bot:**
    Copy the example config and edit it per need.

    **Edit `config.toml`:**
    - Set `homeserver`, `user_id`, and `password` (or use env var `MATRIX_PASSWORD`).
    - Set your `api_key` in the `[gemini]` section.
    - Generate random strings for `pickle_key` and `master_key` (for security).

4.  **Run the bot:**
    ```bash
    go run . -c /path/to/config.toml
    ```

## 🎮 Commands

| Command                  | Description                                       |
| :----------------------- | :------------------------------------------------ |
| `!gemini setkey <key>`   | Set your own personal Gemini API key.             |
| `!gemini enable search`  | Enable Google Search grounding for your requests. |
| `!gemini disable search` | Disable Google Search grounding.                  |
| `!gemini stats`          | Check your token usage and key status.            |
| `!gemini clear`          | Clear your conversation history with the bot.     |

## 📸 Image Analysis

Rakka can analyze images in two ways:

1.  **Direct Upload:** Upload an image with a caption that mentions the bot (e.g., _"@Rakka describe this"_).
2.  **Reply:** Reply to any image in the chat with _"@Rakka analyze this"_.
