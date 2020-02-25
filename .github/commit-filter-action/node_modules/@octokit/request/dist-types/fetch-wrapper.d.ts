import { EndpointInterface } from "@octokit/types";
export default function fetchWrapper(requestOptions: ReturnType<EndpointInterface> & {
    redirect?: string;
}): Promise<{
    status: number;
    url: string;
    headers: {
        [header: string]: string;
    };
    data: any;
}>;
