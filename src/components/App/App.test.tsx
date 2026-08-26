import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { AppRootProps, PluginType } from '@grafana/data';
import { render, waitFor } from '@testing-library/react';
import App from './App';

jest.mock('../../api/client', () => ({
  listConversations: jest.fn().mockResolvedValue([]),
  createConversation: jest.fn(),
  deleteConversation: jest.fn(),
  listMessages: jest.fn(),
  sendChat: jest.fn(),
  streamChat: jest.fn(),
  fetchAgentCard: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  getBackendSrv: () => ({
    fetch: () => Promise.resolve({ data: {} }),
  }),
}));

describe('Components/App', () => {
  let props: AppRootProps;

  beforeEach(() => {
    jest.clearAllMocks();

    props = {
      basename: 'a/sample-app',
      meta: {
        id: 'sample-app',
        name: 'Sample App',
        type: PluginType.app,
        enabled: true,
        jsonData: {},
      },
      query: {},
      path: '',
      onNavChanged: jest.fn(),
    } as unknown as AppRootProps;
  });

  test('renders the chat page without an error', async () => {
    const { queryByText, queryByTestId } = render(
      <MemoryRouter>
        <App {...props} />
      </MemoryRouter>
    );

    // Application is lazy loaded, so we need to wait for the component and routes to be rendered
    await waitFor(() => expect(queryByText(/ask your agent anything/i)).toBeInTheDocument(), { timeout: 2000 });
    expect(queryByTestId('chat-composer-input')).toBeInTheDocument();
  });
});
