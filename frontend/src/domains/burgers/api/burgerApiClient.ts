import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../../auth/storage";

export const burgerApiClient = buildApiClient(getToken);
