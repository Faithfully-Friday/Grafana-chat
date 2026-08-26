import { act, renderHook, waitFor } from '@testing-library/react';
import { useChat } from './useChat';
import * as client from '../api/client';

jest.mock('../api/client');

const mocked = client as jest.Mocked<typeof client>;

describe('useChat', () => {
  beforeEach(() => {
    jest.resetAllMocks();
    mocked.listConversations.mockResolvedValue([]);
    mocked.createConversation.mockResolvedValue({ id: 1, title: 'New chat', updatedAt: '' });
    mocked.listMessages.mockResolvedValue([]);
    mocked.deleteConversation.mockResolvedValue(undefined);
  });

  test('loads conversations on mount', async () => {
    mocked.listConversations.mockResolvedValue([{ id: 7, title: 'Old', updatedAt: '' }]);
    const { result } = renderHook(() => useChat({ streaming: false }));
    await waitFor(() => expect(result.current.conversations).toHaveLength(1));
    expect(result.current.conversations[0].title).toBe('Old');
  });

  test('send (non-streaming) appends user message and full reply', async () => {
    mocked.sendChat.mockResolvedValue('full reply');
    const { result } = renderHook(() => useChat({ streaming: false }));
    await waitFor(() => expect(mocked.listConversations).toHaveBeenCalled());

    await act(async () => {
      await result.current.send('hello');
    });

    expect(result.current.messages).toEqual([
      { role: 'user', content: 'hello' },
      { role: 'assistant', content: 'full reply' },
    ]);
    expect(result.current.sending).toBe(false);
    expect(result.current.activeId).toBe(1);
  });

  test('send (streaming) appends deltas incrementally', async () => {
    mocked.streamChat.mockImplementation(async (_id, _msg, handlers) => {
      handlers.onDelta('str');
      handlers.onDelta('eamed');
    });
    const { result } = renderHook(() => useChat({ streaming: true }));
    await waitFor(() => expect(mocked.listConversations).toHaveBeenCalled());

    await act(async () => {
      await result.current.send('hi');
    });

    expect(result.current.messages[1]).toMatchObject({ role: 'assistant', content: 'streamed', pending: false });
    expect(result.current.sending).toBe(false);
  });

  test('stream error marks the assistant message', async () => {
    mocked.streamChat.mockImplementation(async (_id, _msg, handlers) => {
      handlers.onError('agent exploded');
    });
    const { result } = renderHook(() => useChat({ streaming: true }));
    await waitFor(() => expect(mocked.listConversations).toHaveBeenCalled());

    await act(async () => {
      await result.current.send('hi');
    });

    expect(result.current.messages[1]).toMatchObject({ error: true, content: 'agent exploded' });
  });

  test('selecting a conversation loads its messages', async () => {
    mocked.listMessages.mockResolvedValue([
      { id: 1, role: 'user', content: 'q', createdAt: '' },
      { id: 2, role: 'assistant', content: 'a', createdAt: '' },
    ]);
    const { result } = renderHook(() => useChat({ streaming: false }));
    await waitFor(() => expect(mocked.listConversations).toHaveBeenCalled());

    await act(async () => {
      await result.current.selectConversation(42);
    });

    expect(result.current.activeId).toBe(42);
    expect(result.current.messages).toEqual([
      { role: 'user', content: 'q' },
      { role: 'assistant', content: 'a' },
    ]);
  });

  test('deleting the active conversation clears the view', async () => {
    mocked.listConversations.mockResolvedValue([{ id: 3, title: 'X', updatedAt: '' }]);
    const { result } = renderHook(() => useChat({ streaming: false }));
    await waitFor(() => expect(result.current.conversations).toHaveLength(1));

    await act(async () => {
      await result.current.selectConversation(3);
    });
    await act(async () => {
      await result.current.removeConversation(3);
    });

    expect(result.current.conversations).toHaveLength(0);
    expect(result.current.activeId).toBeNull();
    expect(result.current.messages).toEqual([]);
  });
});
