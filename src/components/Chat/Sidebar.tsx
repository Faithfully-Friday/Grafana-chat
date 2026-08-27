import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Button, Icon, IconButton, ScrollContainer, useStyles2 } from '@grafana/ui';
import { Conversation } from '../../api/client';
import { testIds } from '../testIds';

interface Props {
  conversations: Conversation[];
  activeId: number | null;
  onSelect: (id: number) => void;
  onNew: () => void;
  onDelete: (id: number) => void;
}

export function Sidebar({ conversations, activeId, onSelect, onNew, onDelete }: Props) {
  const s = useStyles2(getStyles);
  return (
    <div className={s.root}>
      <div className={s.header}>
        <Button
          icon="plus"
          variant="secondary"
          fill="outline"
          onClick={onNew}
          fullWidth
          data-testid={testIds.chat.newChatButton}
        >
          New chat
        </Button>
      </div>
      <ScrollContainer>
        <ul className={s.list}>
          {conversations.map((c) => (
            <li key={c.id}>
              <div
                role="button"
                tabIndex={0}
                className={`${s.item} ${c.id === activeId ? s.active : ''}`}
                onClick={() => onSelect(c.id)}
                onKeyDown={(e) => e.key === 'Enter' && onSelect(c.id)}
              >
                <Icon name="comment-alt" className={s.itemIcon} />
                <span className={s.title}>{c.title}</span>
                <IconButton
                  name="trash-alt"
                  size="sm"
                  aria-label={`Delete ${c.title}`}
                  className={s.delete}
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete(c.id);
                  }}
                />
              </div>
            </li>
          ))}
          {conversations.length === 0 && <li className={s.empty}>No conversations yet</li>}
        </ul>
      </ScrollContainer>
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  root: css`
    display: flex;
    flex-direction: column;
    height: 100%;
    background: ${theme.colors.background.secondary};
    border-right: 1px solid ${theme.colors.border.weak};
  `,
  header: css`
    padding: ${theme.spacing(1.5)};
    border-bottom: 1px solid ${theme.colors.border.weak};
  `,
  list: css`
    list-style: none;
    margin: 0;
    padding: ${theme.spacing(1)};
  `,
  item: css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
    padding: ${theme.spacing(1)};
    border-radius: ${theme.shape.radius.default};
    cursor: pointer;
    color: ${theme.colors.text.primary};
    &:hover {
      background: ${theme.colors.action.hover};
    }
    &:hover button {
      visibility: visible;
    }
  `,
  active: css`
    background: ${theme.colors.action.selected};
  `,
  itemIcon: css`
    color: ${theme.colors.text.secondary};
    flex-shrink: 0;
  `,
  title: css`
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  `,
  delete: css`
    visibility: hidden;
  `,
  empty: css`
    padding: ${theme.spacing(2)};
    color: ${theme.colors.text.secondary};
    text-align: center;
  `,
});
