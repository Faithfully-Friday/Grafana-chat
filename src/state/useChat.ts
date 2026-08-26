import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ChatMessage,
  Conversation,
  createConversation,
  deleteConversation,
  listConversations,
  listMessages,
  sendChat,
  streamChat,
} from '../api/client';

export interface UiMessage {
  role: 'user' | 'assistant';
  content: string;
  error?: boolean;
  pending?: boolean;
}

export interface UseChatOptions {
  /** Whether to use the streaming endpoint (from plugin settings). */
  streaming: boolean;
}

export function useChat({ streaming }: UseChatOptions) {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [activeId, setActiveId] = useState<number | null>(null);
  const [messages, setMessages] = useState<UiMessage[]>([]);
  const [sending, setSending] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const refreshConversations = useCallback(async () => {
    const list = await listConversations();
    setConversations(list);
  }, []);

  useEffect(() => {
    refreshConversations().catch(console.error);
  }, [refreshConversations]);

  const selectConversation = useCallback(async (id: number) => {
    abortRef.current?.abort();
    setActiveId(id);
    setLoadingMessages(true);
    try {
      const stored = await listMessages(id);
      setMessages(stored.map((m: ChatMessage) => ({ role: m.role, content: m.content })));
    } catch (e) {
      console.error(e);
      setMessages([]);
    } finally {
      setLoadingMessages(false);
    }
  }, []);

  const newConversation = useCallback(async () => {
    const conv = await createConversation();
    setConversations((prev) => [conv, ...prev]);
    setActiveId(conv.id);
    setMessages([]);
  }, []);

  const removeConversation = useCallback(
    async (id: number) => {
      await deleteConversation(id);
      setConversations((prev) => prev.filter((c) => c.id !== id));
      if (activeId === id) {
        setActiveId(null);
        setMessages([]);
      }
    },
    [activeId]
  );

  const stop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const send = useCallback(
    async (text: string) => {
      const trimmed = text.trim();
      if (!trimmed || sending) {
        return;
      }

      let id = activeId;
      if (id == null) {
        const conv = await createConversation();
        setConversations((prev) => [conv, ...prev]);
        id = conv.id;
        setActiveId(id);
      }

      setSending(true);
      setMessages((prev) => [...prev, { role: 'user', content: trimmed }]);
      // Assistant placeholder; index is captured via length after the user push.
      setMessages((prev) => [...prev, { role: 'assistant', content: '', pending: true }]);

      const patchAssistant = (fn: (m: UiMessage) => UiMessage) =>
        setMessages((prev) => {
          const next = [...prev];
          next[next.length - 1] = fn(next[next.length - 1]);
          return next;
        });

      const finalize = () => {
        setSending(false);
        refreshConversations().catch(console.error);
      };

      if (streaming) {
        const controller = new AbortController();
        abortRef.current = controller;
        let hadError = false;
        try {
          await streamChat(id, trimmed, {
            signal: controller.signal,
            onDelta: (content) =>
              patchAssistant((m) => ({ ...m, pending: false, content: m.content + content })),
            onError: (message) => {
              hadError = true;
              patchAssistant((m) => ({ ...m, pending: false, error: true, content: m.content || message }));
            },
          });
        } catch (e) {
          if (!controller.signal.aborted) {
            hadError = true;
            patchAssistant((m) => ({
              ...m,
              pending: false,
              error: true,
              content: m.content || String(e),
            }));
          }
        } finally {
          abortRef.current = null;
          patchAssistant((m) => ({ ...m, pending: false }));
          if (!hadError) {
            finalize();
          } else {
            setSending(false);
          }
        }
      } else {
        try {
          const reply = await sendChat(id, trimmed);
          patchAssistant(() => ({ role: 'assistant', content: reply }));
          finalize();
        } catch (e) {
          patchAssistant((m) => ({ ...m, pending: false, error: true, content: m.content || String(e) }));
          setSending(false);
        }
      }
    },
    [activeId, sending, streaming, refreshConversations]
  );

  return {
    conversations,
    activeId,
    messages,
    sending,
    loadingMessages,
    selectConversation,
    newConversation,
    removeConversation,
    send,
    stop,
  };
}
