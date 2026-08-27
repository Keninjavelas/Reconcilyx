import { useState } from "react";
import { api } from "@/lib/api";
import type { ChatResponse } from "@/lib/types";
import clsx from "clsx";

interface MessageItem {
  id: string;
  sender: "user" | "ai";
  text: string;
  timestamp: string;
  responsePayload?: ChatResponse;
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
  "Evaluate arbitrage for payment-service",
  "Explain in-place pod resizing",
];

export default function AiChatDrawer({ isOpen, onClose }: AiChatDrawerProps) {
  const [messages, setMessages] = useState<MessageItem[]>([
    {
      id: "welcome",
      sender: "ai",
      text: "👋 Hi! I'm your **Reconcilyx FinOps Assistant**. Ask me about active proposals, cluster carbon emissions, multi-cloud cost arbitrage, or execute pod right-sizing with full safety gates.",
      timestamp: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    },
  ]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);

  if (!isOpen) return null;

  const sendMessage = async (textToSend: string, confirmToken?: string) => {
    if (!textToSend.trim()) return;

    const userMsgId = `msg-${Date.now()}`;
    const userMsg: MessageItem = {
      id: userMsgId,
      sender: "user",
      text: textToSend,
      timestamp: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    };

    setMessages((prev) => [...prev, userMsg]);
    if (!confirmToken) {
      setInput("");
    }
    setLoading(true);

    try {
      const res = await api.chat.send({
        message: textToSend,
        provider: "ollama",
        confirmToken: confirmToken,
      });

      const aiMsg: MessageItem = {
        id: `ai-${Date.now()}`,
        sender: "ai",
        text: res.answer,
        timestamp: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
        responsePayload: res,
        pendingConfirmation: res.confirmationRequired
          ? {
              token: res.confirmationToken ?? "",
              description: res.confirmationDescription ?? res.answer,
              originalQuery: textToSend,
            }
          : undefined,
      };

      setMessages((prev) => [...prev, aiMsg]);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : "Failed to communicate with API server";
      setMessages((prev) => [
        ...prev,
        {
          id: `err-${Date.now()}`,
          sender: "ai",
          text: `⚠️ **Error executing query**\n\n\`${errMsg}\``,
          timestamp: new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const handleConfirmAction = async (msg: MessageItem) => {
    if (!msg.pendingConfirmation) return;
    const { token, originalQuery } = msg.pendingConfirmation;
    // Clear pending state on original message
    setMessages((prev) =>
      prev.map((m) => (m.id === msg.id ? { ...m, pendingConfirmation: undefined } : m)),
    );
    await sendMessage(originalQuery, token);
  };

  const handleCancelAction = (msg: MessageItem) => {
    setMessages((prev) =>
      prev.map((m) =>
        m.id === msg.id
          ? {
              ...m,
              pendingConfirmation: undefined,
              text: m.text + "\n\n❌ *Action cancelled by user.*",
            }
          : m,
      ),
    );
  };

  return (
    <div className="fixed inset-0 z-50 overflow-hidden bg-black/60 backdrop-blur-sm flex justify-end transition-opacity animate-fade-in">
      <div
        className="w-full max-w-lg bg-ink-950 border-l border-white/10 flex flex-col h-full shadow-2xl animate-slide-left"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-6 py-4 border-b border-white/10 flex items-center justify-between bg-ink-900/50">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-mint-400 to-sky-500 flex items-center justify-center text-ink-950 font-bold text-sm">
              🤖
            </div>
            <div>
              <h2 className="font-display text-base tracking-tight font-medium text-white">
                FinOps AI Assistant
              </h2>
              <p className="text-xs text-ink-500">Live Agentic Tool Execution &amp; Safety Gate</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="text-ink-400 hover:text-white p-1.5 rounded-lg hover:bg-white/5 transition"
            aria-label="Close Assistant"
          >
            ✕
          </button>
        </div>

        {/* Message Thread */}
        <div className="flex-1 overflow-y-auto p-6 space-y-4 text-sm">
          {messages.map((m) => (
            <div
              key={m.id}
              className={clsx(
                "flex flex-col max-w-[90%]",
                m.sender === "user" ? "ml-auto items-end" : "mr-auto items-start",
              )}
            >
              <div
                className={clsx(
                  "rounded-2xl px-4 py-3 whitespace-pre-wrap leading-relaxed shadow-sm",
                  m.sender === "user"
                    ? "bg-mint-500/20 border border-mint-500/30 text-mint-100 rounded-br-none"
                    : "bg-white/[0.04] border border-white/10 text-ink-100/90 rounded-bl-none",
                )}
              >
                {m.text}

                {/* Safety Confirmation Prompt Card */}
                {m.pendingConfirmation && (
                  <div className="mt-3 pt-3 border-t border-amber-500/30 bg-amber-500/10 -mx-2 px-3 py-2.5 rounded-xl border">
                    <div className="font-medium text-amber-300 text-xs flex items-center gap-1.5">
                      <span>⚠️</span> Safety Confirmation Required
                    </div>
                    <p className="text-xs text-amber-200/80 mt-1">
                      {m.pendingConfirmation.description}
                    </p>
                    <div className="mt-3 flex items-center gap-2">
                      <button
                        onClick={() => handleConfirmAction(m)}
                        disabled={loading}
                        className="px-3 py-1.5 rounded-lg bg-mint-500 text-ink-950 font-semibold text-xs hover:bg-mint-400 transition"
                      >
                        🚀 Confirm &amp; Execute
                      </button>
                      <button
                        onClick={() => handleCancelAction(m)}
                        disabled={loading}
                        className="px-3 py-1.5 rounded-lg bg-white/10 text-ink-300 font-medium text-xs hover:bg-white/15 transition"
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                )}
              </div>
              <span className="text-[10px] text-ink-500 mt-1 px-1">{m.timestamp}</span>
            </div>
          ))}
          {loading && (
            <div className="flex items-center gap-2 text-xs text-mint-400 animate-pulse">
              <span className="w-2 h-2 rounded-full bg-mint-400" />
              Routing &amp; executing tool call...
            </div>
          )}
        </div>

        {/* Quick Suggestion Chips */}
        <div className="px-6 py-2 border-t border-white/5 flex gap-2 overflow-x-auto no-scrollbar bg-ink-950/80">
          {QUICK_PROMPTS.map((prompt) => (
            <button
              key={prompt}
              onClick={() => sendMessage(prompt)}
              disabled={loading}
              className="text-xs shrink-0 px-3 py-1.5 rounded-full border border-white/10 bg-white/[0.02] text-ink-300 hover:text-mint-300 hover:border-mint-500/30 hover:bg-mint-500/10 transition"
            >
              {prompt}
            </button>
          ))}
        </div>

        {/* Input Form */}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            sendMessage(input);
          }}
          className="p-4 border-t border-white/10 bg-ink-900/50 flex gap-2"
        >
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Ask AI or execute (e.g. approve proposal ...)"
            disabled={loading}
            className="flex-1 bg-white/[0.04] border border-white/10 rounded-xl px-4 py-2.5 text-sm text-white placeholder-ink-500 focus:outline-none focus:border-mint-400 transition"
          />
          <button
            type="submit"
            disabled={loading || !input.trim()}
            className="px-4 py-2.5 rounded-xl bg-mint-500 text-ink-950 font-bold text-sm hover:bg-mint-400 disabled:opacity-50 disabled:pointer-events-none transition"
          >
            Send
          </button>
        </form>
      </div>
    </div>
  );
}
