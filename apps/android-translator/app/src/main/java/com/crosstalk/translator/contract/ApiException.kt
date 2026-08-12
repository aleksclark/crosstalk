package com.crosstalk.translator.contract

/**
 * Domain-facing API failures. Generated OpenAPI exceptions stay inside the adapter.
 */
sealed class ApiException(
    message: String,
    cause: Throwable? = null,
    val statusCode: Int? = null,
) : Exception(message, cause) {
    class Unauthorized(
        message: String = "Unauthorized",
        cause: Throwable? = null,
    ) : ApiException(message, cause, statusCode = 401)

    class Forbidden(
        message: String = "Forbidden",
        cause: Throwable? = null,
    ) : ApiException(message, cause, statusCode = 403)

    class Client(
        message: String,
        statusCode: Int,
        cause: Throwable? = null,
    ) : ApiException(message, cause, statusCode)

    class Server(
        message: String,
        statusCode: Int,
        cause: Throwable? = null,
    ) : ApiException(message, cause, statusCode)

    class Network(
        message: String,
        cause: Throwable? = null,
    ) : ApiException(message, cause)

    class Unexpected(
        message: String,
        cause: Throwable? = null,
    ) : ApiException(message, cause)
}
