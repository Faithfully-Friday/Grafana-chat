import React from 'react';
import { render, screen } from '@testing-library/react';
import { PluginType } from '@grafana/data';
import AppConfig, { AppConfigProps } from './AppConfig';
import { testIds } from 'components/testIds';

jest.mock('../../api/client', () => ({
  fetchAgentCard: jest.fn().mockResolvedValue({ name: 'Test Agent' }),
}));

describe('Components/AppConfig', () => {
  let props: AppConfigProps;

  beforeEach(() => {
    jest.resetAllMocks();

    props = {
      plugin: {
        meta: {
          id: 'sample-app',
          name: 'Sample App',
          type: PluginType.app,
          enabled: true,
          jsonData: {},
        },
      },
      query: {},
    } as unknown as AppConfigProps;
  });

  test('renders the A2A settings fieldset with url, api key, streaming toggle and buttons', () => {
    const plugin = { meta: { ...props.plugin.meta, enabled: false } };

    // @ts-ignore - We don't need to provide `addConfigPage()` and `setChannelSupport()` for these tests
    render(<AppConfig plugin={plugin} query={props.query} />);

    expect(screen.queryByRole('group', { name: /a2a agent settings/i })).toBeInTheDocument();
    expect(screen.queryByTestId(testIds.appConfig.a2aBaseUrl)).toBeInTheDocument();
    expect(screen.queryByTestId(testIds.appConfig.apiKey)).toBeInTheDocument();
    expect(screen.queryByTestId(testIds.appConfig.streamingEnabled)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /save settings/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /test connection/i })).toBeInTheDocument();
  });

  test('submit is disabled until an endpoint URL is entered', () => {
    const plugin = { meta: { ...props.plugin.meta } };

    // @ts-ignore - We don't need to provide `addConfigPage()` and `setChannelSupport()` for these tests
    render(<AppConfig plugin={plugin} query={props.query} />);

    expect(screen.queryByRole('button', { name: /save settings/i })).toBeDisabled();
  });

  test('renders saved values', () => {
    const plugin = {
      meta: {
        ...props.plugin.meta,
        jsonData: { a2aBaseUrl: 'http://agent:8000', streamingEnabled: false },
        secureJsonFields: { apiKey: true },
      },
    };

    // @ts-ignore - We don't need to provide `addConfigPage()` and `setChannelSupport()` for these tests
    render(<AppConfig plugin={plugin} query={props.query} />);

    expect(screen.queryByDisplayValue('http://agent:8000')).toBeInTheDocument();
    expect(screen.queryByTestId(testIds.appConfig.streamingEnabled)).not.toBeChecked();
    expect(screen.queryByRole('button', { name: /save settings/i })).toBeEnabled();
  });
});
