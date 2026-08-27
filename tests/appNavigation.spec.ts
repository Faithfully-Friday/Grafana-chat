import { test, expect } from './fixtures';
import { testIds } from '../src/components/testIds';

test.describe('navigating app', () => {
  test('chat page should render successfully', async ({ gotoPage, page }) => {
    await gotoPage();
    await expect(page.getByText('Ask your agent anything')).toBeVisible();
    await expect(page.getByTestId(testIds.chat.newChatButton)).toBeVisible();
  });
});
