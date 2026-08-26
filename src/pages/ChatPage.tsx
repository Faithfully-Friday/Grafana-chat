import React, { useEffect, useState } from 'react';
import { css } from '@emotion/css';
import { lastValueFrom } from 'rxjs';
import { GrafanaTheme2 } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { IconButton, useStyles2 } from '@grafana/ui';
import pluginJson from '../plugin.json';
import { useChat } from '../state/useChat';
import { Sidebar } from '../components/Chat/Sidebar';
import { MessageList } from '../components/Chat/MessageList';
import { Composer } from '../components/Chat/Composer';

interface PluginSettings {
  jsonData?: {
    streamingEnabled?: boolean;
  };
}

function useStreamingSetting(): boolean {
  const [streaming, setStreaming] = useState(true);
  useEffect(() => {
    lastValueFrom(getBackendSrv().fetch<PluginSettings>({ url: `/api/plugins/${pluginJson.id}/settings`, method: 'GET' }))
      .then((res) => {
        if (res.data.jsonData?.streamingEnabled === false) {
          setStreaming(false);
        }
      })
      .catch(() => {
        // default to streaming on
      });
  }, []);
  return streaming;
}

export function ChatPage() {
  const s = useStyles2(getStyles);
  const streaming = useStreamingSetting();
  const chat = useChat({ streaming });
  const [drawerOpen, setDrawerOpen] = useState(false);

  const sidebar = (
    <Sidebar
      conversations={chat.conversations}
      activeId={chat.activeId}
      onSelect={(id) => {
        chat.selectConversation(id);
        setDrawerOpen(false);
      }}
      onNew={() => {
        chat.newConversation();
        setDrawerOpen(false);
      }}
      onDelete={chat.removeConversation}
    />
  );

  return (
    <div className={s.root}>
      {/* Desktop sidebar */}
      <div className={s.sidebar}>{sidebar}</div>

      {/* Mobile drawer */}
      {drawerOpen && (
        <>
          <div className={s.overlay} onClick={() => setDrawerOpen(false)} />
          <div className={s.drawer}>{sidebar}</div>
        </>
      )}

      <div className={s.chatArea}>
        <div className={s.topBar}>
          <IconButton name="bars" aria-label="Toggle conversations" onClick={() => setDrawerOpen(true)} />
        </div>
        <MessageList messages={chat.messages} loading={chat.loadingMessages} />
        <Composer sending={chat.sending} onSend={chat.send} onStop={chat.stop} />
      </div>
    </div>
  );
}

export default ChatPage;

const getStyles = (theme: GrafanaTheme2) => ({
  root: css`
    display: flex;
    height: 100%;
    width: 100%;
    overflow: hidden;
  `,
  sidebar: css`
    width: 260px;
    flex-shrink: 0;
    display: none;
    ${theme.breakpoints.up('md')} {
      display: block;
    }
  `,
  overlay: css`
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: ${theme.zIndex.modalBackdrop};
    ${theme.breakpoints.up('md')} {
      display: none;
    }
  `,
  drawer: css`
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 280px;
    z-index: ${theme.zIndex.modal};
    ${theme.breakpoints.up('md')} {
      display: none;
    }
  `,
  chatArea: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    height: 100%;
  `,
  topBar: css`
    display: flex;
    align-items: center;
    padding: ${theme.spacing(0.5, 1)};
    border-bottom: 1px solid ${theme.colors.border.weak};
    ${theme.breakpoints.up('md')} {
      display: none;
    }
  `,
});
