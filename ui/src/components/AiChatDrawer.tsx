import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { getLlmConfig } from "@/lib/llm";
import clsx from "clsx";

interface MessageItem {
  id: string;
  sender: "user" | "ai";
  text: string;
  timestamp: string;
  pendingConfirmation?: {
    token: string;
    description: string;
    originalQuery: string;
  };
}

interface AiChatDrawerProps {
  isOpen: boolean;
  onClose: () => void;
}

const QUICK_PROMPTS = [
  "Show pending proposals",
  "What is my carbon footprint?",
  "Which policies are active?",
  "Explain in-place pod resizing",
];

function timeLabel(): string {
  return new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export default function AiChatDrawer({ isOpen, onClose }: AiChatDrawerProps) {
  const [messages, setMessages] = useState<MessageItem[]>([
    {
      id: "welcome",
      sender: "ai",
      text: "Ask about proposals, savings, carbon, cluster registrations, or how a recommendation is evaluated. Tool calls that change cluster state require an explicit confirmation.",
      timestamp: timeLabel(),
    },
  ]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen) endRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, isOpen]);

  if (!isOpen) return null;

  const sendMessage = async (textToSend: string, confirmToken?: string) => {
    if (!textToSend.trim() || loading) return;

    const userMsg: MessageItem = {
      id: `msg-${Date.now()}`,
      sender: "user",
      text: textToSend,
      timestamp: timeLabel(),
    };

    setMessages((prev) => [...prev, userMsg]);
    if (!confirmToken) setInput("");
    setLoading(true);

    try {
      const config = getLlmConfig();
      const res = await api.chat.send({
        message: textToSend,
        provider: config.provider,
        apiKey: config.apiKey || undefined,
        confirmToken,
      });

      setMessages((prev) => [
        ...prev,
        {
          id: `ai-${Date.now()}`,
          sender: "ai",
          text: res.answer,
          timestamp: timeLabel(),
          pendingConfirmation: res.confirmationRequired
            ? {
                token: res.confirmationToken ?? "",
                description: res.confirmationDescription ?? res.answer,
                originalQuery: textToSend,
              }
            : undefined,
        },
      ]);
    } catch (err) {
      setMessages((prev) => [
        ...prev,
        {
          id: `err-${Date.now()}`,
          sender: "ai",
          text: `The control plane could not answer: ${
            err instanceof Error ? err.message : String(err)
          }`,
          timestamp: timeLabel(),
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const handleConfirmAction = async (message: MessageItem) => {
    if (!message.pendingConfirmation) return;
    const { token, originalQuery } = message.pendingConfirmation;
    setMessages((prev) =>
      prev.map((item) => (item.id === message.id ? { ...item, pendingConfirmation: undefined } : item)),
    );
    await sendMessage(originalQuery, token);
  };

  const handleCancelAction = (message: MessageItem) => {
    setMessages((prev) =>
      prev.map((item) =>
        item.id === message.id
          ? {
              ...item,
              pendingConfirmation: undefined,
              text: `${item.text}\n\nAction cancelled. No change was made.`,
            }
          : item,
      ),
    );
  };

  return (
    <div className="overlay fixed inset-0 z-50 flex justify-end" onClick={onClose} role="presentation">
      <aside
        className="flex h-full w-full max-w-lg animate-slide-left flex-col border-l border-white/[0.12] bg-ink-950"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Reconcilyx assistant"
      >
        <header className="flex items-start justify-between gap-4 border-b border-white/[0.1] px-5 py-5">
          <div>
            <div className="eyebrow">Operator assistant</div>
            <h2 className="mt-1 font-display text-[20px] tracking-[-0.03em] text-ink-50">
              Ask the control plane
            </h2>
            <p className="mt-1 text-[11px] text-ink-500">
              Tool routing runs on the server. State changes always require confirmation.
            </p>
          </div>
          <button className="btn-icon" onClick={onClose} aria-label="Close assistant">
            <span aria-hidden>×</span>
          </button>
        </header>

        <div className="flex-1 space-y-5 overflow-y-auto px-5 py-6 scrollbar-thin" aria-live="polite">
          {messages.map((message) => (
            <div
              key={message.id}
              className={clsx("flex flex-col gap-1.5", message.sender === "user" ? "items-end" : "items-start")}
            >
              <div
                className={clsx(
                  "max-w-[92%] whitespace-pre-wrap rounded-2xl px-4 py-3 text-[13px] leading-relaxed",
                  message.sender === "user"
                    ? "rounded-br-sm bg-mint-500 text-ink-950"
                    : "rounded-bl-sm border border-white/[0.1] bg-white/[0.035] text-ink-200",
                )}
              >
                {message.text}

                {message.pendingConfirmation ? (
                  <div className="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/[0.08] px-3 py-3">
                    <div className="font-mono text-[9px] uppercase tracking-[0.16em] text-amber-400">
                      Confirmation required
                    </div>
                    <p className="mt-1 text-[12px] leading-relaxed text-amber-200/90">
                      {message.pendingConfirmation.description}
                    </p>
                    <div className="mt-3 flex gap-2">
                      <button
                        onClick={() => handleConfirmAction(message)}
                        disabled={loading}
                        className="rounded-full bg-mint-500 px-3 py-1.5 text-[11px] font-semibold text-ink-950 transition hover:bg-mint-400 disabled:opacity-40"
                      >
                        Confirm
                      </button>
                      <button
                        onClick={() => handleCancelAction(message)}
                        disabled={loading}
                        className="rounded-full border border-white/[0.14] px-3 py-1.5 text-[11px] text-ink-200 transition hover:border-white/25 disabled:opacity-40"
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : null}
              </div>
              <span className="px-1 font-mono text-[9px] uppercase tracking-[0.14em] text-ink-600">
                {message.sender === "user" ? "You" : "Control plane"} · {message.timestamp}
              </span>
            </div>
          ))}
          {loading ? (
            <div className="flex items-center gap-2 font-mono text-[10px] uppercase tracking-[0.14em] text-ink-500">
              <span className="status-dot bg-mint-500 text-mint-500" aria-hidden />
              Routing tool call
            </div>
          ) : null}
          <div ref={endRef} />
        </div>

        <div className="flex gap-2 overflow-x-auto border-t border-white/[0.08] px-5 py-3 no-scrollbar">
          {QUICK_PROMPTS.map((prompt) => (
            <button
              key={prompt}
              onClick={() => sendMessage(prompt)}
              disabled={loading}
              className="shrink-0 rounded-full border border-white/[0.12] px-3 py-1.5 text-[11px] text-ink-300 transition hover:border-mint-500/40 hover:text-mint-500 disabled:opacity-40"
            >
              {prompt}
            </button>
          ))}
        </div>

        <form
          onSubmit={(event) => {
            event.preventDefault();
            sendMessage(input);
          }}
          className="flex gap-2 border-t border-white/[0.1] p-4"
        >
          <input
            type="text"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            placeholder="Ask a question or request a state change"
            disabled={loading}
            className="input"
            aria-label="Message the control plane assistant"
          />
          <button
            type="submit"
            disabled={loading || !input.trim()}
            className="btn-primary shrink-0 px-4"
          >
            Send
          </button>
        </form>
      </aside>
    </div>
  );
}
