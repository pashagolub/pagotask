# pagotask privacy policy

pagotask is a desktop app that adds tasks to your own Google Tasks account.

- **What it accesses:** only the Google Tasks scope (`https://www.googleapis.com/auth/tasks`). It reads your task list names to find the right list and creates the tasks you type.
- **Where data goes:** directly from your computer to Google's Tasks API. There is no pagotask server; nothing is sent to the author or any third party.
- **What is stored locally:** your Google sign-in token (in Windows Credential Manager), your `config.yaml`, a queue of tasks not yet delivered, and a local log file, all under your user profile. Sign out from the tray menu to delete the token.
- **Revoking access:** at any time at https://myaccount.google.com/permissions.

pagotask's use of information received from Google APIs adheres to the [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), including the Limited Use requirements.

Questions: open an issue at https://github.com/pashagolub/pagotask/issues.
