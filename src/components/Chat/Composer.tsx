import React, { KeyboardEvent, useRef, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Button, useStyles2 } from '@grafana/ui';

interface Props {
  sending: boolean;
  onSend: (text: string) => void;
  onStop: () => void;
}

export function Composer({ sending, onSend, onStop }: Props) {
  const s = useStyles2(getStyles);
  const [value, setValue] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const autosize = () => {
    const el = textareaRef.current;
    if (!el) {
      return;
    }
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  };

  const submit = () => {
    const text = value.trim();
    if (!text || sending) {
      return;
    }
    onSend(text);
    setValue('');
    requestAnimationFrame(autosize);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className={s.root}>
      <div className={s.box}>
        <textarea
          ref={textareaRef}
          className={s.textarea}
          rows={1}
          placeholder="Send a message…  (Enter to send, Shift+Enter for a new line)"
          value={value}
          disabled={sending}
          onChange={(e) => {
            setValue(e.target.value);
            autosize();
          }}
          onKeyDown={onKeyDown}
          data-testid="chat-composer-input"
        />
        {sending ? (
          <Button variant="destructive" icon="square-shape" onClick={onStop} aria-label="Stop generating">
            Stop
          </Button>
        ) : (
          <Button icon="message" onClick={submit} disabled={!value.trim()} aria-label="Send message">
            Send
          </Button>
        )}
      </div>
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  root: css`
    padding: ${theme.spacing(1.5, 2)};
    border-top: 1px solid ${theme.colors.border.weak};
    background: ${theme.colors.background.primary};
  `,
  box: css`
    max-width: 48rem;
    margin: 0 auto;
    display: flex;
    align-items: flex-end;
    gap: ${theme.spacing(1)};
  `,
  textarea: css`
    flex: 1;
    resize: none;
    padding: ${theme.spacing(1, 1.5)};
    border-radius: ${theme.shape.radius.default};
    border: 1px solid ${theme.colors.border.medium};
    background: ${theme.colors.background.canvas};
    color: ${theme.colors.text.primary};
    font-family: inherit;
    font-size: inherit;
    line-height: 1.5;
    &:focus {
      outline: 2px solid ${theme.colors.primary.border};
      outline-offset: -1px;
    }
    &:disabled {
      opacity: 0.6;
    }
  `,
});
