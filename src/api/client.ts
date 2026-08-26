import { lastValueFrom } from 'rxjs';
import { getBackendSrv } from '@grafana/runtime';
import pluginJson from '../plugin.json';

const RESOURCE_BASE = `/api/plugins/${pluginJson.id}/resources`;

export interface Conversation {
  id: number;
  title: string;
  updatedAt: string;
}

export interface ChatMessage {
  id: number;
  role: 'user' | 'assistant';
  content: string;
  createdAt: string;
}

export interface AgentCard {
  name?: string;
  description?: string;
  capabilities?: { streaming?: boolean };
  [key: string]: unknown;
}

export type StreamEventType = 'delta' | 'context' | 'done' | 'error';

export interface StreamEvent {
  type: StreamEventType;
  content?: string;
  message?: string;
  contextId?: string;
}

async function requestJSON<T>(path: string, method: string, data?: unknown): Promise<T> {
  const response = await lastValueFrom(
    getBackendSrv().fetch<T>({
      url: `${RESOURCE_BASE}${path}`,
      method,
      data,
    })
  );
  return response.data;
}

export const listConversations = () => requestJSON<Conversation[]>('/conversations', 'GET');

export const createConversation = () => requestJSON<Conversation>('/conversations', 'POST', {});

export const deleteConversation = (id: number) =>
  requestJSON<void>(`/conversations/${id}`, 'DELETE');

export const listMessages = (conversationId: number) =>
  requestJSON<ChatMessage[]>(`/conversations/${conversationId}/messages`, 'GET');

export const fetchAgentCard = () => requestJSON<AgentCard>('/agent-card', 'GET');

/**
 * Incremental SSE parser. Feed it decoded text chunks; it returns every
 * complete `data:` frame parsed as a StreamEvent. Frames split across chunks
 * are buffered until complete.
 */
export function createSSEParser(onEvent: (ev: StreamEvent) => void): (chunk: string) => void {
  let buffer = '';
  return (chunk: string) => {
    buffer += chunk;
    let idx: number;
    while ((idx = buffer.indexOf('\n\n')) !== -1) {
      const frame = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      const data = frame
        .split('\n')
        .filter((line) => line.startsWith('data:'))
        .map((line) => line.slice(5).trimStart())
        .join('\n');
      if (!data) {
        continue;
      }
      try {
        onEvent(JSON.parse(data) as StreamEvent);
      } catch {
        // skip malformed frames
      }
    }
  };
}

export interface StreamHandlers {
  onDelta: (content: string) => void;
  onError: (message: string) => void;
  signal?: AbortSignal;
}

/**
 * Streams a chat reply from the plugin backend. Uses raw fetch (rather than
 * getBackendSrv) so SSE frames are delivered incrementally via ReadableStream.
 * Resolves when the backend sends `done`; rejects on transport errors.
 */
export async function streamChat(
  conversationId: number,
  message: string,
  handlers: StreamHandlers
): Promise<void> {
  const response = await fetch(`${RESOURCE_BASE}/conversations/${conversationId}/chat`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
    body: JSON.stringify({ message, stream: true }),
    signal: handlers.signal,
  });
  if (!response.ok || !response.body) {
    const text = await response.text().catch(() => '');
    throw new Error(`Chat request failed (HTTP ${response.status}): ${text.slice(0, 200)}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const parse = createSSEParser((ev) => {
    if (ev.type === 'delta' && ev.content) {
      handlers.onDelta(ev.content);
    } else if (ev.type === 'error') {
      handlers.onError(ev.message ?? 'Unknown error');
    }
  });

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    parse(decoder.decode(value, { stream: true }));
  }
  parse(decoder.decode()); // flush
}

/** Non-streaming chat: sends the message and resolves with the full reply. */
export async function sendChat(conversationId: number, message: string): Promise<string> {
  const data = await requestJSON<{ content: string }>(`/conversations/${conversationId}/chat`, 'POST', {
    message,
    stream: false,
  });
  return data.content;
}
