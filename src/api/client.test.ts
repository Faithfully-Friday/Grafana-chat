import { createSSEParser, StreamEvent } from './client';

describe('createSSEParser', () => {
  const collect = () => {
    const events: StreamEvent[] = [];
    return { events, feed: createSSEParser((ev) => events.push(ev)) };
  };

  test('parses a single delta frame', () => {
    const { events, feed } = collect();
    feed('data: {"type":"delta","content":"Hello"}\n\n');
    expect(events).toEqual([{ type: 'delta', content: 'Hello' }]);
  });

  test('parses multiple frames in one chunk', () => {
    const { events, feed } = collect();
    feed('data: {"type":"delta","content":"a"}\n\ndata: {"type":"done"}\n\n');
    expect(events.map((e) => e.type)).toEqual(['delta', 'done']);
  });

  test('buffers frames split across chunks', () => {
    const { events, feed } = collect();
    feed('data: {"type":"del');
    expect(events).toEqual([]);
    feed('ta","content":"wor');
    feed('ld"}\n\n');
    expect(events).toEqual([{ type: 'delta', content: 'world' }]);
  });

  test('skips malformed frames and continues', () => {
    const { events, feed } = collect();
    feed('data: {not json}\n\ndata: {"type":"done"}\n\n');
    expect(events).toEqual([{ type: 'done' }]);
  });

  test('ignores non-data lines and empty frames', () => {
    const { events, feed } = collect();
    feed(': comment\n\nevent: message\ndata: {"type":"delta","content":"x"}\n\n');
    expect(events).toEqual([{ type: 'delta', content: 'x' }]);
  });

  test('parses error frames', () => {
    const { events, feed } = collect();
    feed('data: {"type":"error","message":"boom"}\n\n');
    expect(events).toEqual([{ type: 'error', message: 'boom' }]);
  });
});
