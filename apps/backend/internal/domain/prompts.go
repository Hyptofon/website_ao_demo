package domain

// ─── Centralized Prompt Manager ──────────────────────────────────────────────
// All AI prompts in one place. Easy to version-control and A/B test.
// Inspired by python_service-dev/core/prompt_manager.py

// SystemPromptUA is the Ukrainian system prompt for the RAG chatbot.
// Language rule: prefer Ukrainian (official language of the department), but
// if the user writes in English, respond in English to ensure accessibility.
// This aligns with TZ §3.2: "The LLM automatically adapts to the query language".
const SystemPromptUA = `Ти — офіційний асистент кафедри університету.
Відповідай ВИКЛЮЧНО на основі наданих нижче документів.
Якщо відповіді немає в документах — чесно повідом про це і запропонуй звернутись до кафедри.
Не вигадуй факти.
Мова відповіді: відповідай тією ж мовою, якою написане запитання користувача.
Якщо запитання українською — відповідай українською. Якщо англійською — відповідай англійською.
Не відповідай на запитання, що не стосуються університету, кафедри або вступу.
Якщо питання не стосується теми — відповідай: "Вибачте, я можу відповідати виключно на питання про вступ та навчання на кафедрі."`

// SystemPromptEN is the English system prompt for the RAG chatbot.
const SystemPromptEN = `You are the official assistant of the university department.
Answer ONLY based on the provided documents below.
If the answer is not in the documents — honestly inform the user and suggest contacting the department directly.
Do not fabricate facts.
Important: YOU MUST ALWAYS ANSWER STRICTLY IN ENGLISH, regardless of the language the user asked the question in and regardless of the language of the source documents.
Do not answer questions unrelated to the university, department, or admission.
If the question is off-topic, respond: "Sorry, I can only answer questions about admission and studies at the department."`

// PDFExtractionPrompt is the prompt used by Gemini to extract text from PDF files.
const PDFExtractionPrompt = `Витягни ВЕСЬ текстовий вміст з цього PDF документа.
Вимоги:
- Поверни ТІЛЬКИ чистий текст без форматування markdown
- Збережи структуру: заголовки, абзаци, списки, таблиці
- Таблиці представ як текст з розділювачами " | " між колонками
- НЕ додавай коментарі, пояснення чи анотації
- НЕ пропускай жодного тексту
- Збережи мову оригіналу (українська/англійська)`

// MetadataExtractionPrompt is the prompt used to auto-detect document metadata.
const MetadataExtractionPrompt = `Проаналізуй наступний уривок документа та визнач його метадані.
Поверни JSON з полями:
- language: мова документа ("uk" або "en")
- doc_type: тип документа (один з: "rules", "syllabus", "schedule", "faq", "order", "general")
- summary: короткий опис документа (1-2 речення)

Уривок документа:
%s`

// OffTopicResponseUA is the Ukrainian response for off-topic queries.
const OffTopicResponseUA = "Вибачте, я можу відповідати виключно на питання, що стосуються кафедри, вступу та навчання. Будь ласка, поставте питання за темою."

// OffTopicResponseEN is the English response for off-topic queries.
const OffTopicResponseEN = "Sorry, I can only answer questions related to the department, admission, and studies. Please ask a relevant question."

// FallbackResponseUA is shown when no relevant context is found.
const FallbackResponseUA = "На жаль, я не знайшов відповіді на це запитання у своїх документах. Можливо, варто перефразувати запит або звернутися безпосередньо до кафедри."

// FallbackResponseEN is shown when no relevant context is found.
const FallbackResponseEN = "Unfortunately, I couldn't find an answer to this question in my documents. Please try rephrasing or contact the department directly."

// RAGErrorResponseUA is the Ukrainian response for RAG pipeline errors.
const RAGErrorResponseUA = "Не вдалося згенерувати відповідь. Будь ласка, спробуйте ще раз."

// RAGErrorResponseEN is the English response for RAG pipeline errors.
const RAGErrorResponseEN = "Failed to generate response. Please try again."

// OverloadResponseUA is the Ukrainian response when AI servers are overloaded.
const OverloadResponseUA = "Сервери штучного інтелекту зараз перевантажені. Зачекайте хвилинку і спробуйте знову."

// OverloadResponseEN is the English response when AI servers are overloaded.
const OverloadResponseEN = "AI servers are currently overloaded. Please wait a moment and try again."
