---
title: Outlook
sidebar_position: 3
---

# Microsoft Email

If you have a personal Outlook or legacy Hotmail account, you won't be able to authenticate with Matcha normally using OAuth without some special configuration. In this guide, we will create a [Microsoft Entra][1] App, give that app the required scopes and permissions, and then authenticate to that app with Matcha's OAuth tool.

[1]: https://www.microsoft.com/en-us/security/business/microsoft-entra

# Microsoft Entra

In this section, we will create a Microsoft Entra App with the required scopes and permissions.

### Azure Account

First, create a [free](https://azure.microsoft.com/free/?WT.mc_id=A261C142F) Azure account affiliated with your personal Outlook account. You will have to enter credit card details, but your account will not be charged.

### Tenant

A tenant is a collection of user identities, apps, and groups that are managed by Microsoft Entra ID. It's a secure boundary that controls access to an organization's resources. Once your Azure account is created, a default tenant called **Default Directory** will be created in your Entra Portal.

You can check your Tenants by going to the top-right of the [Microsoft Entra Admin Center](https://entra.microsoft.com/#home) and seeing the name of the tenant right below your username.

### Register an Application

Now, we will [register an app][3] with the Microsoft Identity platform.

[3]: https://learn.microsoft.com/en-us/entra/identity-platform/quickstart-register-app?tabs=certificate

1. Sign in to the **Microsoft Entra admin center** as at least a *Cloud Application Administrator*.
2. If you have access to multiple tenants, use the Settings icon in the top menu to switch to the tenant in which you want to register the application from the Directories + subscriptions menu.
3. Browse to **Identity > Applications > App registrations** and select **New registration**.
4. Enter a display Name for your application -- call it something like `Matcha{YourUsername}`.
5. Under **Who can use this application or access this API?**, select **Accounts in any organizational directory (Any Microsoft Entra ID tenant - Multitenant) and personal Microsoft accounts (e.g. Skype, Xbox)** -- this will allow your application to use the `common` tenant when authenticating later.

### Giving the App Scopes

We must give our App the required scope permissions to access user's email through IMAP and SMTP.

In the left menu, go to **App Registrations > All Applications > Your App**. Then, under the **Api Permissions** menu...

1. Select **Add a Permission > Microsoft Graph API** and select the following scopes under **Delegated permissions**:

   - `email`
   - `offline_access`
   - `User.Read`
   - `Mail.ReadWrite`
   - `Mail.Send`
   - `IMAP.AccessAsUser.All`
   - `SMTP.Send`

2. Click **Grant admin consent for Default Directory** to grant consent for the scopes being added to the app.

### Platform Configuration

Now, we will register our app as a Web application to be able to use it as an authentication endpoint later.

At the left App Menu, go to **Authentication > Platform Configurations > Add a platform** and select the `Web` platform. This is important because it will enable us to pass in a client secret to our app's endpoint.

#### Redirect URIs

Add the following Redirect URI to the Web platform:

- `http://localhost:8189`

### Setting Up a Client Secret

We need a Client Secret in order to use OAuth. Click on **Certificates and Secrets > Client Secrets > New Client Secret** and follow the directives to create a new client secret.

Record the value of your client secret and keep it safe, because it won't be shown to you later. Your client secret will also expire on the date that you set it to, so you will have to repeat this process at that time.

Finally, go to **Overview** and record the value of your **Application (client) ID**.

# Using Matcha

We are now ready to authenticate to our newly-created App.

### 1. Save your client credentials

Create the file `~/.config/matcha/oauth_client_outlook.json`:

```json
{
  "client_id": "YOUR_APPLICATION_CLIENT_ID",
  "client_secret": "YOUR_CLIENT_SECRET_VALUE"
}
```

### 2. Authentication

Run `matcha oauth auth yourname@email.com` and go to the `http://localhost:8189` as prompted. Authenticate with your personal Outlook account, and authorize your Entra app to access your account.

Once authorized, you'll see "Authorization complete!" in your terminal. A token will be stored that lets you authenticate via XOauth2.

Or, if you don't want to create the JSON file, run with inline credentials:

```bash
matcha oauth auth your@outlook.com --provider outlook --client-id YOUR_ID --client-secret YOUR_SECRET
```

### Enabling IMAP in Outlook

We are almost ready to connect with Matcha. First, we must enable IMAP access in Outlook if you have not done so already.

Go to [outlook.com](https://outlook.com) and access your personal Inbox. Then:

1. Select **Settings > Mail > Forwarding and IMAP**.
2. Under POP and IMAP, toggle the slider for **Let devices and apps use IMAP**.
3. Select Save.

### 3. Add your account in Matcha

From Matcha, open settings and choose to add a new account. Enter:

- **Provider**: outlook
- **Display name**: The name that will appear on emails you send
- **Username**: Your Outlook email address
- **Email Address**: The email address to fetch messages from (usually the same)
- **Auth Method**: oauth2

No password is needed — Matcha will use the tokens from the authorization step.

### Managing OAuth tokens

```bash
# Get a fresh access token (auto-refreshes if expired)
matcha oauth token your@outlook.com

# Revoke and delete stored tokens
matcha oauth revoke your@outlook.com

# Re-authorize
matcha oauth auth your@outlook.com
```

---

## Alternative: App Password

If you prefer not to set up OAuth2, you can use an app password. App passwords are available for Microsoft accounts with two-step verification enabled.

### 1. Enable two-step verification

1. Go to [https://account.microsoft.com/security](https://account.microsoft.com/security).
2. Under **Two-step verification**, click **Turn on** if not already enabled.

### 2. Create an App Password

1. Go to [https://account.live.com/proofs/manage/additional](https://account.live.com/proofs/manage/additional).
2. Under **App passwords**, click **Create a new app password**.
3. Copy the generated password.

### 3. Add your account in Matcha

From Matcha, open settings and choose to add a new account. Enter:

- **Provider**: outlook
- **Display name**: The name that will appear on emails you send
- **Username**: Your Outlook email address
- **Email Address**: The email address to fetch messages from
- **Password**: The generated app password (not your regular Microsoft password)

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| **Authentication error with parser message** | Outlook's IMAP error responses can trip up some IMAP parsers. Use OAuth2 instead of app passwords if you see wire-parsing errors. |
| **OAuth2: consent screen not showing permissions** | Ensure you added the correct API permissions (IMAP.AccessAsUser.All, SMTP.Send, offline_access) in Azure. |
| **OAuth2: token expired** | Run `matcha oauth auth your@outlook.com` to re-authorize. |
| **OAuth2: refresh failed** | Your client secret may have expired. Create a new one in Azure and update `oauth_client_outlook.json`. |
| **"python3 not found"** | OAuth2 requires Python 3. Install it via your package manager. |
| **App password not available** | App passwords require two-step verification to be enabled on your Microsoft account. |
