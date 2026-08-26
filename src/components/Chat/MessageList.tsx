import React, { useEffect, useRef } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Icon, useStyles2 } from '@grafana/ui';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { UiMessage } from '../../state/useChat';

interface Props {
  messages: UiMessage[];
  loading: boolean;
}

export function MessageList({ messages, loading }: Props) {
  const s = useStyles2(getStyles);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  // Keep the view pinned to the bottom unless the user scrolled up.
  const pinnedRef = useRef(true);

  const onScroll = () => {
    const el = containerRef.current;
    if (!el) {
      return;
    }
    pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };

  useEffect(() => {
    if (pinnedRef.current) {
      bottomRef.current?.scrollIntoView?.({ block: 'end' });
    }
  }, [messages]);

  if (loading) {
    return (
      <div className={s.center} ref={containerRef}>
        <Icon name="fa fa-spinner" className="fa-spin" /> Loading…
      </div>
    );
  }

  return (
    <div className={s.scroll} ref={containerRef} onScroll={onScroll}>
      <div className={s.column}>
        {messages.length === 0 && (
          <div className={s.center}>
            <h3>Ask your agent anything</h3>
            <p className={s.muted}>Messages are stored on the server and the agent remembers the conversation.</p>
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} className={m.role === 'user' ? s.rowUser : s.rowAssistant}>
            <div className={`${s.bubble} ${m.role === 'user' ? s.userBubble : s.assistantBubble} ${m.error ? s.errorBubble : ''}`}>
              {m.role === 'assistant' ? (
                <>
                  {m.content ? (
                    <div className={s.markdown}>
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>{m.content}</ReactMarkdown>
                    </div>
                  ) : null}
                  {m.pending && <span className={s.cursor}>▍</span>}
                </>
              ) : (
                m.content
              )}
            </div>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  scroll: css`
    flex: 1;
    overflow-y: auto;
    min-height: 0;
  `,
  column: css`
    max-width: 48rem;
    margin: 0 auto;
    padding: ${theme.spacing(2)};
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(1.5)};
  `,
  center: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: ${theme.spacing(1)};
    text-align: center;
    padding: ${theme.spacing(4)};
  `,
  muted: css`
    color: ${theme.colors.text.secondary};
  `,
  rowUser: css`
    display: flex;
    justify-content: flex-end;
  `,
  rowAssistant: css`
    display: flex;
    justify-content: flex-start;
  `,
  bubble: css`
    padding: ${theme.spacing(1, 1.5)};
    border-radius: ${theme.shape.radius.default};
    max-width: 85%;
    white-space: pre-wrap;
    word-break: break-word;
  `,
  userBubble: css`
    background: ${theme.colors.primary.main};
    color: ${theme.colors.primary.contrastText};
  `,
  assistantBubble: css`
    background: ${theme.colors.background.secondary};
    white-space: normal;
  `,
  errorBubble: css`
    border: 1px solid ${theme.colors.error.border};
    color: ${theme.colors.error.text};
  `,
  cursor: css`
    animation: blink 1s steps(1) infinite;
    @keyframes blink {
      50% {
        opacity: 0;
      }
    }
  `,
  markdown: css`
    & p {
      margin: 0 0 ${theme.spacing(1)};
    }
    & p:last-child {
      margin-bottom: 0;
    }
    & pre {
      background: ${theme.colors.background.canvas};
      padding: ${theme.spacing(1)};
      border-radius: ${theme.shape.radius.default};
      overflow-x: auto;
    }
    & code {
      font-family: ${theme.typography.fontFamilyMonospace};
      font-size: ${theme.typography.bodySmall.fontSize};
    }
    & table {
      border-collapse: collapse;
    }
    & th,
    & td {
      border: 1px solid ${theme.colors.border.weak};
      padding: ${theme.spacing(0.5, 1)};
    }
  `,
});
