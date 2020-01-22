# Registration and signing in

To use the features of [Netdata Cloud](README.md), you must first register an account with Netdata Cloud and associate your first Netdata node with the Netdata Cloud [registry](../../registry/README.md). **Netdata Cloud is entirely free for all Netdata users**, and does not store any metrics created by your machines. You keep your data—Netdata Cloud just connects it all together.

!!! attention "Opting-in to Netdata Cloud"
    By [signing in](signing-in.md) to Netdata Cloud, you opt-in to let Netdata Cloud receive and store the information described [here](../../registry/README.md#what-data-does-the-registry-store). We never store the metrics collected by Netdata agents, just machine GUIDs, person GUID, URLs, and account information.

## Registering a Netdata Cloud account

There is only one prerequisite to using Netdata Cloud: A working Netdata agent. If you don't have a running Netdata agent yet, check out the [installation guides](../../packaging/installer/) for more information.

To begin, visit the web dashboard of your Netdata agent by navigating your browser of choice to `http://SERVER-IP:19999`. You’ll see a dashboard much like this:

![A screenshot of Netdata's web interface](https://user-images.githubusercontent.com/1153921/59644657-b7330300-9122-11e9-9dda-ea784422f3f2.png)

From here, you need to register for a Netdata Cloud account. Click on the **Sign in** button on the top-right corner of the dashboard's view.

![A screenshot of the Sign in button in the Netdata dashboard](https://user-images.githubusercontent.com/1153921/59782688-6252d200-9273-11e9-9975-52be0d6714bf.png)

??? note "Alternative registration routes"
    While we recommend the **Sign in** button, the Netdata dashboard has one other direct route registering for or signing in to a Netdata Cloud account.

```
The text **Please sign in to netdata.cloud to view your nodes!** contains a link to access Netdata Cloud.

![A screenshot of the Netdata Cloud sign in link](https://user-images.githubusercontent.com/1153921/59644958-2f4df880-9124-11e9-946c-bb30c8735e0a.png)

Two other routes exist, but they are more directly related to accessing the Nodes View. They will, however, require either registration or sign in and thus are valid routes to access Netdata Cloud.

One route can be found in the **Nodes Beta** button the left side of the navigation menu:

![A screenshot of a link to the Nodes View in Netdata Cloud](https://user-images.githubusercontent.com/1153921/59644663-c1ed9800-9122-11e9-9ebc-d67e7db229a7.png)

A second route can be found in the Nodes List—the drop-down menu in the top-left corner of the Netdata dashboard:

 ![A screenshot of a second link to the Nodes View in Netdata Cloud](https://user-images.githubusercontent.com/1153921/59644973-3d9c1480-9124-11e9-9a1d-33c412578a9f.png)
```

??? note "Registration route when using a private registry"
    If you're using a private registry, clicking the **Sign in** button will display a modal window warning you about the process of migrating away from your private registry and to Netdata Cloud's registry.

```
![A screenshot of the private registry warning modal](https://user-images.githubusercontent.com/1153921/59782901-ca091d00-9273-11e9-9f9a-0cb18f78ca26.png)

If you agree to use Netdata Cloud over your private registry, and opt-in to let Netdata Cloud receive and store the information described [here](../../registry/README.md#what-data-does-the-registry-store), you should click the **Sign in** button again. If not, click the **Cancel** button to continue using your private registry.
```

### Choosing your registration or sign in method

After clicking the **Sign in** button, you'll be directed to the Netdata Cloud registration/sign in page. Choose to authorize with your Google account, GitHub account, or email.

!!! attention
    Be consistent with the sign in method you use, whether GitHub, Google, or email. If you sign in via different methods, the system will create multiple Netdata Cloud accounts, one for each sign-in method used. We plan to offer multiple authentication methods for the same account in the future.

![Screenshot of the registration/sign in view for Netdata Cloud](https://user-images.githubusercontent.com/1153921/59783226-8bc02d80-9274-11e9-8bbc-4718759b3145.png)

### Registration via Google

Click the **Authorize with Google** button to begin registration. You will be redirected to a Google authentication form where you confirm you will "share your name, email address, language preference, and profile picture with netdata.cloud." 

![Screenshot of the Google authentication screen for Netdata Cloud](https://user-images.githubusercontent.com/1153921/59786094-50752d00-927b-11e9-9411-5d7ce2b71ab0.png)

Click on the account you would like to connect to Netdata Cloud to continue and then skip down to [Visiting Netdata Cloud for the first time](#visiting-the-nodes-view-for-the-first-time) for further instructions.

### Registration via GitHub

Click the **Authorize with GitHub** button to begin registration. You will be redirected to a GitHub authentication form where you confirm to share your email address with Netdata Cloud to create your account.

![Screenshot of the GitHub authentication screen for Netdata Cloud](https://user-images.githubusercontent.com/1153921/59786227-a2b64e00-927b-11e9-939b-6fc51ef453b0.png)

Click the **Authorize Netdata** button to continue and then skip down to [Visiting Netdata Cloud for the first time](#visiting-the-nodes-view-for-the-first-time) for further instructions.

### Registration via email

Enter your preferred email into the field and click the **Authorize** button. 

Open your email account and check for the verification email—it should arrive in less than a minute. If it doesn't show up, check your spam folder or click the **Resend email** button in the Netdata Cloud interface.

When the email arrives, open it and click on the green **Sign in** button and then skip down to [Visiting Netdata Cloud for the first time](#visiting-the-nodes-view-for-the-first-time) for further instructions.

![Screenshot of the verification email](https://user-images.githubusercontent.com/1153921/59783969-338a2b00-9276-11e9-84b8-a4f678de1242.png)

## Visiting the Nodes View for the first time

Regardless of which sign in method you used, you'll now be redirected back to your Netdata agent's dashboard. This node has now been associated with your Netdata Cloud account. Netdata Cloud uses a list of nodes associated with your account to populate the Nodes List dropdown in the dashboard and the Nodes View feature of Netdata Cloud.

**For more information on how to use the Nodes View, visit the [Nodes View guide](nodes-view.md).**

## Signing in to your Netdata Cloud account

The process of signing in to an existing Netdata Cloud account the same as [registering for a new account](#registering-a-netdata-cloud-account). The recommended method is to use the **Sign in** button at the top-right corner of a Netdata nodes's dashboard. Choose the method you used to register for your Netdata Cloud account and complete the process.

![A screenshot of the Sign in button in the Netdata dashboard](https://user-images.githubusercontent.com/1153921/59782688-6252d200-9273-11e9-9975-52be0d6714bf.png)

## Adding additional nodes to your Netdata Cloud account

There is currently only one way to associate additional Netdata nodes with your Netdata Cloud account: You must visit the web dashboard for each node and click the **Sign in** button and complete the [sign in process](#signing-in-to-your-netdata-cloud-account).

!!! note ""
    We are aware that the process of registering each node individually is cumbersome for those who want to implement Netdata Cloud's features across a large infrastructure. 

```
Please view [this comment on issue #6318](https://github.com/netdata/netdata/issues/6318#issuecomment-504106329) for how we plan on improving the process for adding additional nodes to your Netdata Cloud account.
```

## Private registries and Netdata Cloud

If you use a [private registry](../../registry/README.md#run-your-own-registry), and sign in to Netdata Cloud, you'll be using the Netdata Cloud registry in addition to your private registry.

Clicking the **Sign in** button on the Netdata dashboard will display a modal window warning you about the synchronization of your private registry's entries to the Netdata Cloud's registry.

![A screenshot of the private registry warning modal](https://user-images.githubusercontent.com/1153921/59807493-fd1bd280-92ac-11e9-8017-98efb2cbbed8.png)

If your company's data policies don't allow storing information about your nodes on the Netdata Cloud registry, you should click the **Cancel** button and continue using your private registry. You'll be able to access the Nodes List in the top-left corner of a Netdata dashboard, but you won't be able to use the [Nodes View](nodes-view.md) feature within Netdata Cloud, or any of the [additional features](https://blog.netdata.cloud/posts/netdata-cloud-announcement/#what-features-will-netdata-cloud-offer) on our roadmap. You can also sign up for the waiting list for the [hosted and/or on-premises versions of Netdata Cloud](README.md#running-netdata-cloud-on-premises-or-as-a-hosted-instance) that we're working on.

If you agree to use Netdata Cloud over your private registry, and opt-in to let Netdata Cloud receive and store the information described [here](../../registry/README.md#what-data-does-the-registry-store), you should click the **Sign in** button again to continue the registration/sign in process.

### Returning to your private registry

If you register for or sign in to Netdata Cloud from a node previously associated with a private registry, you can easily return to your private registry by signing out.

You can sign out in two ways:

1.  **From a node's dashboard**: In the top-right corner you will find a dropdown menu with your email address. Click that and then click the **Sign Out** button.
2.  **From Netdata Cloud**: Click on your profile picture in the top-right corner and then click on the **Sign Out** button.

Signing out from Netdata Cloud and returning to your private registry *does not remove* the [information stored](../../registry/README.md#what-data-does-the-registry-store) about your nodes or account details.

But, upon signout, your Nodes List on all dashboards will once more be populated by your private registry and not Netdata Cloud.

<!-- ## The 'Synchronize with Netdata Cloud' button

Once signed in to Netdata Cloud, the Nodes List dropdown will now show a button labeled `Synchronize with netdata.cloud`. 

The `Synchronize with Netdata Cloud` button is a migration (or import) tool for Netdata Cloud. If either the public or your private registry contains a list of nodes associated with your `person_guid`, it will import them into Netdata Cloud and associate them with the `accounts` information in the Netdata Cloud registry.

When you click the `Synchronize with netdata.cloud` button, you will receive one of two popup messages based on whether you were using the public registry (at `registry.my-netdata.io`) or a private registry.

**Public registry**:

![Screenshot of the synchronization warning for public registries](https://user-images.githubusercontent.com/1153921/59807540-3a806000-92ad-11e9-99b7-e2254d817ed4.png)

**Private registry**:

![Screenshot of the synchronization warning for private registries](https://user-images.githubusercontent.com/1153921/59807459-d8bff600-92ac-11e9-997f-e84b909f266e.png)

If you do not want to synchronize your registry of choice with Netdata Cloud, click `Cancel`.

If you do, click `Synchronize`. This will push GUIDs, hostnames, and URLs to Netdata Cloud's registry.

Now, when you visit the Nodes View, you will be able to see all the nodes that were once associated with the public/private registry you were using previously. -->

## What's next?

Learn how to use the [Nodes View](nodes-view.md) to monitor many nodes concurrently.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fnetdata-cloud%2Fsigning-in&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
