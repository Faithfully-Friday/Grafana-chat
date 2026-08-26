import React, { ChangeEvent, useState } from 'react';
import { lastValueFrom } from 'rxjs';
import { css } from '@emotion/css';
import { AppPluginMeta, GrafanaTheme2, PluginConfigPageProps, PluginMeta } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { Alert, Button, Field, FieldSet, Input, SecretInput, Switch, useStyles2 } from '@grafana/ui';
import { testIds } from '../testIds';
import { AgentCard, fetchAgentCard } from '../../api/client';

type AppPluginSettings = {
  a2aBaseUrl?: string;
  streamingEnabled?: boolean;
};

type State = {
  // The A2A JSON-RPC endpoint of the agent.
  a2aBaseUrl: string;
  // Tells us if the API key secret is set.
  isApiKeySet: boolean;
  // Bearer token for the A2A endpoint.
  apiKey: string;
  // Whether to use message/stream (SSE) instead of blocking message/send.
  streamingEnabled: boolean;
};

type TestResult =
  | { status: 'idle' }
  | { status: 'testing' }
  | { status: 'success'; card: AgentCard }
  | { status: 'error'; message: string };

export interface AppConfigProps extends PluginConfigPageProps<AppPluginMeta<AppPluginSettings>> {}

const AppConfig = ({ plugin }: AppConfigProps) => {
  const s = useStyles2(getStyles);
  const { enabled, pinned, jsonData, secureJsonFields } = plugin.meta;
  const [state, setState] = useState<State>({
    a2aBaseUrl: jsonData?.a2aBaseUrl || '',
    apiKey: '',
    isApiKeySet: Boolean(secureJsonFields?.apiKey),
    streamingEnabled: jsonData?.streamingEnabled !== false,
  });
  const [testResult, setTestResult] = useState<TestResult>({ status: 'idle' });

  const isSubmitDisabled = Boolean(!state.a2aBaseUrl);

  const onResetApiKey = () =>
    setState({
      ...state,
      apiKey: '',
      isApiKeySet: false,
    });

  const onChange = (event: ChangeEvent<HTMLInputElement>) => {
    setState({
      ...state,
      [event.target.name]: event.target.value.trim(),
    });
  };

  const onSubmit = () => {
    if (isSubmitDisabled) {
      return;
    }

    updatePluginAndReload(plugin.meta.id, {
      enabled,
      pinned,
      jsonData: {
        a2aBaseUrl: state.a2aBaseUrl,
        streamingEnabled: state.streamingEnabled,
      },
      // This cannot be queried later by the frontend.
      // We don't want to override it in case it was set previously and left untouched now.
      secureJsonData: state.isApiKeySet
        ? undefined
        : {
            apiKey: state.apiKey,
          },
    });
  };

  // The backend reads persisted settings, so this only works after saving.
  const onTestConnection = async () => {
    setTestResult({ status: 'testing' });
    try {
      const card = await fetchAgentCard();
      setTestResult({ status: 'success', card });
    } catch (e: any) {
      const message = e?.data?.message ?? e?.statusText ?? String(e);
      setTestResult({ status: 'error', message });
    }
  };

  return (
    <form onSubmit={onSubmit}>
      <FieldSet label="A2A Agent Settings">
        <Field label="Agent endpoint URL" description="The A2A JSON-RPC endpoint of your agent, e.g. http://localhost:8000 or a LangGraph deployment's /a2a/{assistant_id} URL">
          <Input
            width={60}
            name="a2aBaseUrl"
            id="config-a2a-base-url"
            data-testid={testIds.appConfig.a2aBaseUrl}
            value={state.a2aBaseUrl}
            placeholder="E.g.: http://localhost:8000"
            onChange={onChange}
          />
        </Field>

        <Field label="API Key" description="Optional bearer token for authenticating to the agent" className={s.marginTop}>
          <SecretInput
            width={60}
            id="config-api-key"
            data-testid={testIds.appConfig.apiKey}
            name="apiKey"
            value={state.apiKey}
            isConfigured={state.isApiKeySet}
            placeholder={'Your API key'}
            onChange={onChange}
            onReset={onResetApiKey}
          />
        </Field>

        <Field
          label="Stream responses"
          description="Stream tokens as they are generated (message/stream). Disable for agents that only support blocking message/send."
          className={s.marginTop}
        >
          <Switch
            id="config-streaming"
            data-testid={testIds.appConfig.streamingEnabled}
            name="streamingEnabled"
            value={state.streamingEnabled}
            onChange={(e) => setState({ ...state, streamingEnabled: e.currentTarget.checked })}
          />
        </Field>

        <div className={s.marginTop}>
          <Button type="submit" data-testid={testIds.appConfig.submit} disabled={isSubmitDisabled}>
            Save settings
          </Button>
          <Button
            type="button"
            variant="secondary"
            className={s.marginLeft}
            data-testid={testIds.appConfig.testConnection}
            disabled={testResult.status === 'testing'}
            onClick={onTestConnection}
          >
            {testResult.status === 'testing' ? 'Testing…' : 'Test connection'}
          </Button>
        </div>

        {testResult.status === 'success' && (
          <Alert title="Connected" severity="success" className={s.marginTop} data-testid={testIds.appConfig.testConnectionSuccess}>
            {testResult.card.name ? `Agent: ${testResult.card.name}` : 'Agent card fetched successfully.'}
            {testResult.card.description ? ` — ${testResult.card.description}` : ''}
            {testResult.card.capabilities && (
              <> — streaming: {testResult.card.capabilities.streaming ? 'supported' : 'not supported'}</>
            )}
          </Alert>
        )}
        {testResult.status === 'error' && (
          <Alert title="Connection failed" severity="error" className={s.marginTop} data-testid={testIds.appConfig.testConnectionError}>
            {testResult.message}. Make sure you saved the settings first — the backend reads the persisted values.
          </Alert>
        )}
      </FieldSet>
    </form>
  );
};

export default AppConfig;

const getStyles = (theme: GrafanaTheme2) => ({
  colorWeak: css`
    color: ${theme.colors.text.secondary};
  `,
  marginTop: css`
    margin-top: ${theme.spacing(3)};
  `,
  marginLeft: css`
    margin-left: ${theme.spacing(1)};
  `,
});

const updatePluginAndReload = async (pluginId: string, data: Partial<PluginMeta<AppPluginSettings>>) => {
  try {
    await updatePlugin(pluginId, data);

    // Reloading the page as the changes made here wouldn't be propagated to the actual plugin otherwise.
    // This is not ideal, however unfortunately currently there is no supported way for updating the plugin state.
    window.location.reload();
  } catch (e) {
    console.error('Error while updating the plugin', e);
  }
};

const updatePlugin = async (pluginId: string, data: Partial<PluginMeta>) => {
  const response = await getBackendSrv().fetch({
    url: `/api/plugins/${pluginId}/settings`,
    method: 'POST',
    data,
  });

  return lastValueFrom(response);
};
