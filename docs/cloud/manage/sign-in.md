# Sign in to Netdata

This page explains how to sign in to Netdata with your email, Google account, or GitHub account, and provides some tips if you're having trouble signing in.

You can [sign in to Netdata](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_first_section) through one of three methods: email, Google, or GitHub. Email uses a
time-sensitive link that authenticates your browser, and Google/GitHub both use OAuth to associate your email address
with a Netdata Cloud account.

No matter the method, your Netdata Cloud account is based around your email address. Netdata Cloud does not store
passwords.

## Email

To sign in with email, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_email_section), enter your email address, and click
the **Sign in by email** button.

![Verify your email!](https://user-images.githubusercontent.com/82235632/125475486-c667635a-067f-4866-9411-9f7f795a0d50.png)

Click the **Verify** button in the email to begin using Netdata Cloud.

To use this same Netdata Cloud account on additional devices, request another sign in email, open the email on that
device, and sign in.

### Don't have a Netdata Cloud account yet?

If you don't have a Netdata Cloud account yet you won't need to worry about it. During the sign in process we will create one for you and make the process seamless to you.

After your account is created and you sign in to Netdata, you first are asked to agree to Netdata Cloud's [Privacy
Policy](https://www.netdata.cloud/privacy/) and [Terms of Use](https://www.netdata.cloud/terms/). Once you agree with these you are directed
through the Netdata Cloud onboarding process, which is explained in the [Netdata Cloud
quickstart](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md).

### Troubleshooting

You should receive your sign in email in less than a minute. The subject is **Verify your email!** for new sign-ups, **Sign in to Netdata** for sign ins.
The sender is `no-reply@netdata.cloud` via `sendgrid.net`.

If you don't see the email, try the following:

- Check your spam folder.
- In Gmail, check the **Updates** category.
- Check [Netdata Cloud status](https://status.netdata.cloud) for ongoing issues with our infrastructure.
- Request another sign in email via the [sign in page](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_troubleshooting_section).

You may also want to add `no-reply@netdata.cloud` to your address book or contacts list, especially if you're using
a public email service, such as Gmail. You may also want to whitelist/allowlist either the specific email or the entire
`netdata.cloud` domain.

In some cases, temporary issues with your mail server or email account may result in your email address being added to a Bounce list by Sendgrid.
If you are added to that list, no Netdata cloud email can reach you, including alarm notifications. Let us know in Discord that you have trouble receiving
any email from us and someone will ask you to provide your email address privately, so we can check if you are on the Bounce list.

## Google and GitHub OAuth

When you use Google/GitHub OAuth, your Netdata Cloud account is associated with the email address that Netdata Cloud
receives via OAuth.

To sign in with Google or GitHub OAuth, visit [Netdata Cloud](https://app.netdata.cloud/sign-in?cloudRoute=spaces?utm_source=docs&utm_content=sign_in_button_google_github_section) and click the
**Continue with Google/GitHub** or button. Enter your Google/GitHub username and your password. Complete two-factor
authentication if you or your organization has it enabled.

You are then signed in to Netdata Cloud or directed to the new-user onboarding if you have not signed up previously.

## Reset a password

Netdata Cloud does not store passwords and does not support password resets. All of our sign in methods do not
require passwords, and use either links in emails or Google/GitHub OAuth for authentication.

## Switch between sign in methods

You can switch between sign in methods if the email account associated with each method is the same.

For example, you first sign in via your email account, `user@example.com`, and later sign out. You later attempt to sign
in via a GitHub account associated with `user@example.com`. Netdata Cloud recognizes that the two are the same and signs
you in to your original account.

However, if you first sign in via your `user@example.com` email account and then sign in via a Google account associated
with `user2@example.com`, Netdata Cloud creates a new account and begins the onboarding process.

It is not currently possible to link an account created with `user@example.com` to a Google account associated with
`user2@example.com`.
