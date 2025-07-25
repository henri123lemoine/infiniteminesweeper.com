// Common aliases
var $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
var $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

$root.ChunkID = (function() {

    /**
     * Properties of a ChunkID.
     * @exports IChunkID
     * @interface IChunkID
     * @property {number|null} [x] ChunkID x
     * @property {number|null} [y] ChunkID y
     */

    /**
     * Constructs a new ChunkID.
     * @exports ChunkID
     * @classdesc Represents a ChunkID.
     * @implements IChunkID
     * @constructor
     * @param {IChunkID=} [properties] Properties to set
     */
    function ChunkID(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * ChunkID x.
     * @member {number} x
     * @memberof ChunkID
     * @instance
     */
    ChunkID.prototype.x = 0;

    /**
     * ChunkID y.
     * @member {number} y
     * @memberof ChunkID
     * @instance
     */
    ChunkID.prototype.y = 0;

    /**
     * Creates a new ChunkID instance using the specified properties.
     * @function create
     * @memberof ChunkID
     * @static
     * @param {IChunkID=} [properties] Properties to set
     * @returns {ChunkID} ChunkID instance
     */
    ChunkID.create = function create(properties) {
        return new ChunkID(properties);
    };

    /**
     * Encodes the specified ChunkID message. Does not implicitly {@link ChunkID.verify|verify} messages.
     * @function encode
     * @memberof ChunkID
     * @static
     * @param {IChunkID} message ChunkID message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ChunkID.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.x != null && Object.hasOwnProperty.call(message, "x"))
            writer.uint32(/* id 1, wireType 0 =*/8).int32(message.x);
        if (message.y != null && Object.hasOwnProperty.call(message, "y"))
            writer.uint32(/* id 2, wireType 0 =*/16).int32(message.y);
        return writer;
    };

    /**
     * Encodes the specified ChunkID message, length delimited. Does not implicitly {@link ChunkID.verify|verify} messages.
     * @function encodeDelimited
     * @memberof ChunkID
     * @static
     * @param {IChunkID} message ChunkID message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ChunkID.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a ChunkID message from the specified reader or buffer.
     * @function decode
     * @memberof ChunkID
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {ChunkID} ChunkID
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ChunkID.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.ChunkID();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.x = reader.int32();
                    break;
                }
            case 2: {
                    message.y = reader.int32();
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a ChunkID message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof ChunkID
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {ChunkID} ChunkID
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ChunkID.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a ChunkID message.
     * @function verify
     * @memberof ChunkID
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    ChunkID.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.x != null && message.hasOwnProperty("x"))
            if (!$util.isInteger(message.x))
                return "x: integer expected";
        if (message.y != null && message.hasOwnProperty("y"))
            if (!$util.isInteger(message.y))
                return "y: integer expected";
        return null;
    };

    /**
     * Creates a ChunkID message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof ChunkID
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {ChunkID} ChunkID
     */
    ChunkID.fromObject = function fromObject(object) {
        if (object instanceof $root.ChunkID)
            return object;
        var message = new $root.ChunkID();
        if (object.x != null)
            message.x = object.x | 0;
        if (object.y != null)
            message.y = object.y | 0;
        return message;
    };

    /**
     * Creates a plain object from a ChunkID message. Also converts values to other types if specified.
     * @function toObject
     * @memberof ChunkID
     * @static
     * @param {ChunkID} message ChunkID
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    ChunkID.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.x = 0;
            object.y = 0;
        }
        if (message.x != null && message.hasOwnProperty("x"))
            object.x = message.x;
        if (message.y != null && message.hasOwnProperty("y"))
            object.y = message.y;
        return object;
    };

    /**
     * Converts this ChunkID to JSON.
     * @function toJSON
     * @memberof ChunkID
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    ChunkID.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for ChunkID
     * @function getTypeUrl
     * @memberof ChunkID
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    ChunkID.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/ChunkID";
    };

    return ChunkID;
})();

$root.RevealRequest = (function() {

    /**
     * Properties of a RevealRequest.
     * @exports IRevealRequest
     * @interface IRevealRequest
     * @property {IChunkID|null} [chunkId] RevealRequest chunkId
     * @property {number|null} [x] RevealRequest x
     * @property {number|null} [y] RevealRequest y
     */

    /**
     * Constructs a new RevealRequest.
     * @exports RevealRequest
     * @classdesc Represents a RevealRequest.
     * @implements IRevealRequest
     * @constructor
     * @param {IRevealRequest=} [properties] Properties to set
     */
    function RevealRequest(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * RevealRequest chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof RevealRequest
     * @instance
     */
    RevealRequest.prototype.chunkId = null;

    /**
     * RevealRequest x.
     * @member {number} x
     * @memberof RevealRequest
     * @instance
     */
    RevealRequest.prototype.x = 0;

    /**
     * RevealRequest y.
     * @member {number} y
     * @memberof RevealRequest
     * @instance
     */
    RevealRequest.prototype.y = 0;

    /**
     * Creates a new RevealRequest instance using the specified properties.
     * @function create
     * @memberof RevealRequest
     * @static
     * @param {IRevealRequest=} [properties] Properties to set
     * @returns {RevealRequest} RevealRequest instance
     */
    RevealRequest.create = function create(properties) {
        return new RevealRequest(properties);
    };

    /**
     * Encodes the specified RevealRequest message. Does not implicitly {@link RevealRequest.verify|verify} messages.
     * @function encode
     * @memberof RevealRequest
     * @static
     * @param {IRevealRequest} message RevealRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    RevealRequest.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.x != null && Object.hasOwnProperty.call(message, "x"))
            writer.uint32(/* id 2, wireType 0 =*/16).int32(message.x);
        if (message.y != null && Object.hasOwnProperty.call(message, "y"))
            writer.uint32(/* id 3, wireType 0 =*/24).int32(message.y);
        return writer;
    };

    /**
     * Encodes the specified RevealRequest message, length delimited. Does not implicitly {@link RevealRequest.verify|verify} messages.
     * @function encodeDelimited
     * @memberof RevealRequest
     * @static
     * @param {IRevealRequest} message RevealRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    RevealRequest.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a RevealRequest message from the specified reader or buffer.
     * @function decode
     * @memberof RevealRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {RevealRequest} RevealRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    RevealRequest.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.RevealRequest();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.x = reader.int32();
                    break;
                }
            case 3: {
                    message.y = reader.int32();
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a RevealRequest message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof RevealRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {RevealRequest} RevealRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    RevealRequest.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a RevealRequest message.
     * @function verify
     * @memberof RevealRequest
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    RevealRequest.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        if (message.x != null && message.hasOwnProperty("x"))
            if (!$util.isInteger(message.x))
                return "x: integer expected";
        if (message.y != null && message.hasOwnProperty("y"))
            if (!$util.isInteger(message.y))
                return "y: integer expected";
        return null;
    };

    /**
     * Creates a RevealRequest message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof RevealRequest
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {RevealRequest} RevealRequest
     */
    RevealRequest.fromObject = function fromObject(object) {
        if (object instanceof $root.RevealRequest)
            return object;
        var message = new $root.RevealRequest();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".RevealRequest.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        if (object.x != null)
            message.x = object.x | 0;
        if (object.y != null)
            message.y = object.y | 0;
        return message;
    };

    /**
     * Creates a plain object from a RevealRequest message. Also converts values to other types if specified.
     * @function toObject
     * @memberof RevealRequest
     * @static
     * @param {RevealRequest} message RevealRequest
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    RevealRequest.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.chunkId = null;
            object.x = 0;
            object.y = 0;
        }
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        if (message.x != null && message.hasOwnProperty("x"))
            object.x = message.x;
        if (message.y != null && message.hasOwnProperty("y"))
            object.y = message.y;
        return object;
    };

    /**
     * Converts this RevealRequest to JSON.
     * @function toJSON
     * @memberof RevealRequest
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    RevealRequest.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for RevealRequest
     * @function getTypeUrl
     * @memberof RevealRequest
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    RevealRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/RevealRequest";
    };

    return RevealRequest;
})();

$root.SubscribeRequest = (function() {

    /**
     * Properties of a SubscribeRequest.
     * @exports ISubscribeRequest
     * @interface ISubscribeRequest
     * @property {IChunkID|null} [chunkId] SubscribeRequest chunkId
     */

    /**
     * Constructs a new SubscribeRequest.
     * @exports SubscribeRequest
     * @classdesc Represents a SubscribeRequest.
     * @implements ISubscribeRequest
     * @constructor
     * @param {ISubscribeRequest=} [properties] Properties to set
     */
    function SubscribeRequest(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * SubscribeRequest chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof SubscribeRequest
     * @instance
     */
    SubscribeRequest.prototype.chunkId = null;

    /**
     * Creates a new SubscribeRequest instance using the specified properties.
     * @function create
     * @memberof SubscribeRequest
     * @static
     * @param {ISubscribeRequest=} [properties] Properties to set
     * @returns {SubscribeRequest} SubscribeRequest instance
     */
    SubscribeRequest.create = function create(properties) {
        return new SubscribeRequest(properties);
    };

    /**
     * Encodes the specified SubscribeRequest message. Does not implicitly {@link SubscribeRequest.verify|verify} messages.
     * @function encode
     * @memberof SubscribeRequest
     * @static
     * @param {ISubscribeRequest} message SubscribeRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    SubscribeRequest.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified SubscribeRequest message, length delimited. Does not implicitly {@link SubscribeRequest.verify|verify} messages.
     * @function encodeDelimited
     * @memberof SubscribeRequest
     * @static
     * @param {ISubscribeRequest} message SubscribeRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    SubscribeRequest.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a SubscribeRequest message from the specified reader or buffer.
     * @function decode
     * @memberof SubscribeRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {SubscribeRequest} SubscribeRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    SubscribeRequest.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.SubscribeRequest();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a SubscribeRequest message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof SubscribeRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {SubscribeRequest} SubscribeRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    SubscribeRequest.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a SubscribeRequest message.
     * @function verify
     * @memberof SubscribeRequest
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    SubscribeRequest.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        return null;
    };

    /**
     * Creates a SubscribeRequest message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof SubscribeRequest
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {SubscribeRequest} SubscribeRequest
     */
    SubscribeRequest.fromObject = function fromObject(object) {
        if (object instanceof $root.SubscribeRequest)
            return object;
        var message = new $root.SubscribeRequest();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".SubscribeRequest.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        return message;
    };

    /**
     * Creates a plain object from a SubscribeRequest message. Also converts values to other types if specified.
     * @function toObject
     * @memberof SubscribeRequest
     * @static
     * @param {SubscribeRequest} message SubscribeRequest
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    SubscribeRequest.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults)
            object.chunkId = null;
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        return object;
    };

    /**
     * Converts this SubscribeRequest to JSON.
     * @function toJSON
     * @memberof SubscribeRequest
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    SubscribeRequest.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for SubscribeRequest
     * @function getTypeUrl
     * @memberof SubscribeRequest
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    SubscribeRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/SubscribeRequest";
    };

    return SubscribeRequest;
})();

$root.UnsubscribeRequest = (function() {

    /**
     * Properties of an UnsubscribeRequest.
     * @exports IUnsubscribeRequest
     * @interface IUnsubscribeRequest
     * @property {IChunkID|null} [chunkId] UnsubscribeRequest chunkId
     */

    /**
     * Constructs a new UnsubscribeRequest.
     * @exports UnsubscribeRequest
     * @classdesc Represents an UnsubscribeRequest.
     * @implements IUnsubscribeRequest
     * @constructor
     * @param {IUnsubscribeRequest=} [properties] Properties to set
     */
    function UnsubscribeRequest(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * UnsubscribeRequest chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof UnsubscribeRequest
     * @instance
     */
    UnsubscribeRequest.prototype.chunkId = null;

    /**
     * Creates a new UnsubscribeRequest instance using the specified properties.
     * @function create
     * @memberof UnsubscribeRequest
     * @static
     * @param {IUnsubscribeRequest=} [properties] Properties to set
     * @returns {UnsubscribeRequest} UnsubscribeRequest instance
     */
    UnsubscribeRequest.create = function create(properties) {
        return new UnsubscribeRequest(properties);
    };

    /**
     * Encodes the specified UnsubscribeRequest message. Does not implicitly {@link UnsubscribeRequest.verify|verify} messages.
     * @function encode
     * @memberof UnsubscribeRequest
     * @static
     * @param {IUnsubscribeRequest} message UnsubscribeRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    UnsubscribeRequest.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified UnsubscribeRequest message, length delimited. Does not implicitly {@link UnsubscribeRequest.verify|verify} messages.
     * @function encodeDelimited
     * @memberof UnsubscribeRequest
     * @static
     * @param {IUnsubscribeRequest} message UnsubscribeRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    UnsubscribeRequest.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes an UnsubscribeRequest message from the specified reader or buffer.
     * @function decode
     * @memberof UnsubscribeRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {UnsubscribeRequest} UnsubscribeRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    UnsubscribeRequest.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.UnsubscribeRequest();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes an UnsubscribeRequest message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof UnsubscribeRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {UnsubscribeRequest} UnsubscribeRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    UnsubscribeRequest.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies an UnsubscribeRequest message.
     * @function verify
     * @memberof UnsubscribeRequest
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    UnsubscribeRequest.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        return null;
    };

    /**
     * Creates an UnsubscribeRequest message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof UnsubscribeRequest
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {UnsubscribeRequest} UnsubscribeRequest
     */
    UnsubscribeRequest.fromObject = function fromObject(object) {
        if (object instanceof $root.UnsubscribeRequest)
            return object;
        var message = new $root.UnsubscribeRequest();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".UnsubscribeRequest.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        return message;
    };

    /**
     * Creates a plain object from an UnsubscribeRequest message. Also converts values to other types if specified.
     * @function toObject
     * @memberof UnsubscribeRequest
     * @static
     * @param {UnsubscribeRequest} message UnsubscribeRequest
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    UnsubscribeRequest.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults)
            object.chunkId = null;
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        return object;
    };

    /**
     * Converts this UnsubscribeRequest to JSON.
     * @function toJSON
     * @memberof UnsubscribeRequest
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    UnsubscribeRequest.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for UnsubscribeRequest
     * @function getTypeUrl
     * @memberof UnsubscribeRequest
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    UnsubscribeRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/UnsubscribeRequest";
    };

    return UnsubscribeRequest;
})();

$root.SeedRequest = (function() {

    /**
     * Properties of a SeedRequest.
     * @exports ISeedRequest
     * @interface ISeedRequest
     * @property {IChunkID|null} [chunkId] SeedRequest chunkId
     */

    /**
     * Constructs a new SeedRequest.
     * @exports SeedRequest
     * @classdesc Represents a SeedRequest.
     * @implements ISeedRequest
     * @constructor
     * @param {ISeedRequest=} [properties] Properties to set
     */
    function SeedRequest(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * SeedRequest chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof SeedRequest
     * @instance
     */
    SeedRequest.prototype.chunkId = null;

    /**
     * Creates a new SeedRequest instance using the specified properties.
     * @function create
     * @memberof SeedRequest
     * @static
     * @param {ISeedRequest=} [properties] Properties to set
     * @returns {SeedRequest} SeedRequest instance
     */
    SeedRequest.create = function create(properties) {
        return new SeedRequest(properties);
    };

    /**
     * Encodes the specified SeedRequest message. Does not implicitly {@link SeedRequest.verify|verify} messages.
     * @function encode
     * @memberof SeedRequest
     * @static
     * @param {ISeedRequest} message SeedRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    SeedRequest.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified SeedRequest message, length delimited. Does not implicitly {@link SeedRequest.verify|verify} messages.
     * @function encodeDelimited
     * @memberof SeedRequest
     * @static
     * @param {ISeedRequest} message SeedRequest message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    SeedRequest.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a SeedRequest message from the specified reader or buffer.
     * @function decode
     * @memberof SeedRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {SeedRequest} SeedRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    SeedRequest.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.SeedRequest();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a SeedRequest message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof SeedRequest
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {SeedRequest} SeedRequest
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    SeedRequest.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a SeedRequest message.
     * @function verify
     * @memberof SeedRequest
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    SeedRequest.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        return null;
    };

    /**
     * Creates a SeedRequest message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof SeedRequest
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {SeedRequest} SeedRequest
     */
    SeedRequest.fromObject = function fromObject(object) {
        if (object instanceof $root.SeedRequest)
            return object;
        var message = new $root.SeedRequest();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".SeedRequest.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        return message;
    };

    /**
     * Creates a plain object from a SeedRequest message. Also converts values to other types if specified.
     * @function toObject
     * @memberof SeedRequest
     * @static
     * @param {SeedRequest} message SeedRequest
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    SeedRequest.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults)
            object.chunkId = null;
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        return object;
    };

    /**
     * Converts this SeedRequest to JSON.
     * @function toJSON
     * @memberof SeedRequest
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    SeedRequest.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for SeedRequest
     * @function getTypeUrl
     * @memberof SeedRequest
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    SeedRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/SeedRequest";
    };

    return SeedRequest;
})();

$root.SeedResponse = (function() {

    /**
     * Properties of a SeedResponse.
     * @exports ISeedResponse
     * @interface ISeedResponse
     * @property {IChunkID|null} [chunkId] SeedResponse chunkId
     * @property {number|Long|null} [seed] SeedResponse seed
     */

    /**
     * Constructs a new SeedResponse.
     * @exports SeedResponse
     * @classdesc Represents a SeedResponse.
     * @implements ISeedResponse
     * @constructor
     * @param {ISeedResponse=} [properties] Properties to set
     */
    function SeedResponse(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * SeedResponse chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof SeedResponse
     * @instance
     */
    SeedResponse.prototype.chunkId = null;

    /**
     * SeedResponse seed.
     * @member {number|Long} seed
     * @memberof SeedResponse
     * @instance
     */
    SeedResponse.prototype.seed = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

    /**
     * Creates a new SeedResponse instance using the specified properties.
     * @function create
     * @memberof SeedResponse
     * @static
     * @param {ISeedResponse=} [properties] Properties to set
     * @returns {SeedResponse} SeedResponse instance
     */
    SeedResponse.create = function create(properties) {
        return new SeedResponse(properties);
    };

    /**
     * Encodes the specified SeedResponse message. Does not implicitly {@link SeedResponse.verify|verify} messages.
     * @function encode
     * @memberof SeedResponse
     * @static
     * @param {ISeedResponse} message SeedResponse message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    SeedResponse.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.seed != null && Object.hasOwnProperty.call(message, "seed"))
            writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.seed);
        return writer;
    };

    /**
     * Encodes the specified SeedResponse message, length delimited. Does not implicitly {@link SeedResponse.verify|verify} messages.
     * @function encodeDelimited
     * @memberof SeedResponse
     * @static
     * @param {ISeedResponse} message SeedResponse message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    SeedResponse.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a SeedResponse message from the specified reader or buffer.
     * @function decode
     * @memberof SeedResponse
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {SeedResponse} SeedResponse
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    SeedResponse.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.SeedResponse();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.seed = reader.uint64();
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a SeedResponse message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof SeedResponse
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {SeedResponse} SeedResponse
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    SeedResponse.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a SeedResponse message.
     * @function verify
     * @memberof SeedResponse
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    SeedResponse.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        if (message.seed != null && message.hasOwnProperty("seed"))
            if (!$util.isInteger(message.seed) && !(message.seed && $util.isInteger(message.seed.low) && $util.isInteger(message.seed.high)))
                return "seed: integer|Long expected";
        return null;
    };

    /**
     * Creates a SeedResponse message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof SeedResponse
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {SeedResponse} SeedResponse
     */
    SeedResponse.fromObject = function fromObject(object) {
        if (object instanceof $root.SeedResponse)
            return object;
        var message = new $root.SeedResponse();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".SeedResponse.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        if (object.seed != null)
            if ($util.Long)
                (message.seed = $util.Long.fromValue(object.seed)).unsigned = true;
            else if (typeof object.seed === "string")
                message.seed = parseInt(object.seed, 10);
            else if (typeof object.seed === "number")
                message.seed = object.seed;
            else if (typeof object.seed === "object")
                message.seed = new $util.LongBits(object.seed.low >>> 0, object.seed.high >>> 0).toNumber(true);
        return message;
    };

    /**
     * Creates a plain object from a SeedResponse message. Also converts values to other types if specified.
     * @function toObject
     * @memberof SeedResponse
     * @static
     * @param {SeedResponse} message SeedResponse
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    SeedResponse.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.chunkId = null;
            if ($util.Long) {
                var long = new $util.Long(0, 0, true);
                object.seed = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
            } else
                object.seed = options.longs === String ? "0" : 0;
        }
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        if (message.seed != null && message.hasOwnProperty("seed"))
            if (typeof message.seed === "number")
                object.seed = options.longs === String ? String(message.seed) : message.seed;
            else
                object.seed = options.longs === String ? $util.Long.prototype.toString.call(message.seed) : options.longs === Number ? new $util.LongBits(message.seed.low >>> 0, message.seed.high >>> 0).toNumber(true) : message.seed;
        return object;
    };

    /**
     * Converts this SeedResponse to JSON.
     * @function toJSON
     * @memberof SeedResponse
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    SeedResponse.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for SeedResponse
     * @function getTypeUrl
     * @memberof SeedResponse
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    SeedResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/SeedResponse";
    };

    return SeedResponse;
})();

$root.Reveal = (function() {

    /**
     * Properties of a Reveal.
     * @exports IReveal
     * @interface IReveal
     * @property {IChunkID|null} [chunkId] Reveal chunkId
     * @property {number|null} [x] Reveal x
     * @property {number|null} [y] Reveal y
     * @property {number|null} [playerId] Reveal playerId
     */

    /**
     * Constructs a new Reveal.
     * @exports Reveal
     * @classdesc Represents a Reveal.
     * @implements IReveal
     * @constructor
     * @param {IReveal=} [properties] Properties to set
     */
    function Reveal(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * Reveal chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof Reveal
     * @instance
     */
    Reveal.prototype.chunkId = null;

    /**
     * Reveal x.
     * @member {number} x
     * @memberof Reveal
     * @instance
     */
    Reveal.prototype.x = 0;

    /**
     * Reveal y.
     * @member {number} y
     * @memberof Reveal
     * @instance
     */
    Reveal.prototype.y = 0;

    /**
     * Reveal playerId.
     * @member {number} playerId
     * @memberof Reveal
     * @instance
     */
    Reveal.prototype.playerId = 0;

    /**
     * Creates a new Reveal instance using the specified properties.
     * @function create
     * @memberof Reveal
     * @static
     * @param {IReveal=} [properties] Properties to set
     * @returns {Reveal} Reveal instance
     */
    Reveal.create = function create(properties) {
        return new Reveal(properties);
    };

    /**
     * Encodes the specified Reveal message. Does not implicitly {@link Reveal.verify|verify} messages.
     * @function encode
     * @memberof Reveal
     * @static
     * @param {IReveal} message Reveal message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Reveal.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.x != null && Object.hasOwnProperty.call(message, "x"))
            writer.uint32(/* id 2, wireType 0 =*/16).int32(message.x);
        if (message.y != null && Object.hasOwnProperty.call(message, "y"))
            writer.uint32(/* id 3, wireType 0 =*/24).int32(message.y);
        if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
            writer.uint32(/* id 4, wireType 0 =*/32).int32(message.playerId);
        return writer;
    };

    /**
     * Encodes the specified Reveal message, length delimited. Does not implicitly {@link Reveal.verify|verify} messages.
     * @function encodeDelimited
     * @memberof Reveal
     * @static
     * @param {IReveal} message Reveal message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Reveal.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a Reveal message from the specified reader or buffer.
     * @function decode
     * @memberof Reveal
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {Reveal} Reveal
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Reveal.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.Reveal();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.x = reader.int32();
                    break;
                }
            case 3: {
                    message.y = reader.int32();
                    break;
                }
            case 4: {
                    message.playerId = reader.int32();
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a Reveal message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof Reveal
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {Reveal} Reveal
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Reveal.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a Reveal message.
     * @function verify
     * @memberof Reveal
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    Reveal.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        if (message.x != null && message.hasOwnProperty("x"))
            if (!$util.isInteger(message.x))
                return "x: integer expected";
        if (message.y != null && message.hasOwnProperty("y"))
            if (!$util.isInteger(message.y))
                return "y: integer expected";
        if (message.playerId != null && message.hasOwnProperty("playerId"))
            if (!$util.isInteger(message.playerId))
                return "playerId: integer expected";
        return null;
    };

    /**
     * Creates a Reveal message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof Reveal
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {Reveal} Reveal
     */
    Reveal.fromObject = function fromObject(object) {
        if (object instanceof $root.Reveal)
            return object;
        var message = new $root.Reveal();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".Reveal.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        if (object.x != null)
            message.x = object.x | 0;
        if (object.y != null)
            message.y = object.y | 0;
        if (object.playerId != null)
            message.playerId = object.playerId | 0;
        return message;
    };

    /**
     * Creates a plain object from a Reveal message. Also converts values to other types if specified.
     * @function toObject
     * @memberof Reveal
     * @static
     * @param {Reveal} message Reveal
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    Reveal.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.chunkId = null;
            object.x = 0;
            object.y = 0;
            object.playerId = 0;
        }
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        if (message.x != null && message.hasOwnProperty("x"))
            object.x = message.x;
        if (message.y != null && message.hasOwnProperty("y"))
            object.y = message.y;
        if (message.playerId != null && message.hasOwnProperty("playerId"))
            object.playerId = message.playerId;
        return object;
    };

    /**
     * Converts this Reveal to JSON.
     * @function toJSON
     * @memberof Reveal
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    Reveal.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for Reveal
     * @function getTypeUrl
     * @memberof Reveal
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    Reveal.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/Reveal";
    };

    return Reveal;
})();

$root.ChunkSync = (function() {

    /**
     * Properties of a ChunkSync.
     * @exports IChunkSync
     * @interface IChunkSync
     * @property {IChunkID|null} [chunkId] ChunkSync chunkId
     * @property {number|Long|null} [seed] ChunkSync seed
     * @property {Array.<IReveal>|null} [reveals] ChunkSync reveals
     */

    /**
     * Constructs a new ChunkSync.
     * @exports ChunkSync
     * @classdesc Represents a ChunkSync.
     * @implements IChunkSync
     * @constructor
     * @param {IChunkSync=} [properties] Properties to set
     */
    function ChunkSync(properties) {
        this.reveals = [];
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * ChunkSync chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof ChunkSync
     * @instance
     */
    ChunkSync.prototype.chunkId = null;

    /**
     * ChunkSync seed.
     * @member {number|Long} seed
     * @memberof ChunkSync
     * @instance
     */
    ChunkSync.prototype.seed = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

    /**
     * ChunkSync reveals.
     * @member {Array.<IReveal>} reveals
     * @memberof ChunkSync
     * @instance
     */
    ChunkSync.prototype.reveals = $util.emptyArray;

    /**
     * Creates a new ChunkSync instance using the specified properties.
     * @function create
     * @memberof ChunkSync
     * @static
     * @param {IChunkSync=} [properties] Properties to set
     * @returns {ChunkSync} ChunkSync instance
     */
    ChunkSync.create = function create(properties) {
        return new ChunkSync(properties);
    };

    /**
     * Encodes the specified ChunkSync message. Does not implicitly {@link ChunkSync.verify|verify} messages.
     * @function encode
     * @memberof ChunkSync
     * @static
     * @param {IChunkSync} message ChunkSync message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ChunkSync.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.seed != null && Object.hasOwnProperty.call(message, "seed"))
            writer.uint32(/* id 2, wireType 0 =*/16).uint64(message.seed);
        if (message.reveals != null && message.reveals.length)
            for (var i = 0; i < message.reveals.length; ++i)
                $root.Reveal.encode(message.reveals[i], writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified ChunkSync message, length delimited. Does not implicitly {@link ChunkSync.verify|verify} messages.
     * @function encodeDelimited
     * @memberof ChunkSync
     * @static
     * @param {IChunkSync} message ChunkSync message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ChunkSync.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a ChunkSync message from the specified reader or buffer.
     * @function decode
     * @memberof ChunkSync
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {ChunkSync} ChunkSync
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ChunkSync.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.ChunkSync();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.seed = reader.uint64();
                    break;
                }
            case 3: {
                    if (!(message.reveals && message.reveals.length))
                        message.reveals = [];
                    message.reveals.push($root.Reveal.decode(reader, reader.uint32()));
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a ChunkSync message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof ChunkSync
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {ChunkSync} ChunkSync
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ChunkSync.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a ChunkSync message.
     * @function verify
     * @memberof ChunkSync
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    ChunkSync.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        if (message.seed != null && message.hasOwnProperty("seed"))
            if (!$util.isInteger(message.seed) && !(message.seed && $util.isInteger(message.seed.low) && $util.isInteger(message.seed.high)))
                return "seed: integer|Long expected";
        if (message.reveals != null && message.hasOwnProperty("reveals")) {
            if (!Array.isArray(message.reveals))
                return "reveals: array expected";
            for (var i = 0; i < message.reveals.length; ++i) {
                var error = $root.Reveal.verify(message.reveals[i]);
                if (error)
                    return "reveals." + error;
            }
        }
        return null;
    };

    /**
     * Creates a ChunkSync message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof ChunkSync
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {ChunkSync} ChunkSync
     */
    ChunkSync.fromObject = function fromObject(object) {
        if (object instanceof $root.ChunkSync)
            return object;
        var message = new $root.ChunkSync();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".ChunkSync.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        if (object.seed != null)
            if ($util.Long)
                (message.seed = $util.Long.fromValue(object.seed)).unsigned = true;
            else if (typeof object.seed === "string")
                message.seed = parseInt(object.seed, 10);
            else if (typeof object.seed === "number")
                message.seed = object.seed;
            else if (typeof object.seed === "object")
                message.seed = new $util.LongBits(object.seed.low >>> 0, object.seed.high >>> 0).toNumber(true);
        if (object.reveals) {
            if (!Array.isArray(object.reveals))
                throw TypeError(".ChunkSync.reveals: array expected");
            message.reveals = [];
            for (var i = 0; i < object.reveals.length; ++i) {
                if (typeof object.reveals[i] !== "object")
                    throw TypeError(".ChunkSync.reveals: object expected");
                message.reveals[i] = $root.Reveal.fromObject(object.reveals[i]);
            }
        }
        return message;
    };

    /**
     * Creates a plain object from a ChunkSync message. Also converts values to other types if specified.
     * @function toObject
     * @memberof ChunkSync
     * @static
     * @param {ChunkSync} message ChunkSync
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    ChunkSync.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.arrays || options.defaults)
            object.reveals = [];
        if (options.defaults) {
            object.chunkId = null;
            if ($util.Long) {
                var long = new $util.Long(0, 0, true);
                object.seed = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
            } else
                object.seed = options.longs === String ? "0" : 0;
        }
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        if (message.seed != null && message.hasOwnProperty("seed"))
            if (typeof message.seed === "number")
                object.seed = options.longs === String ? String(message.seed) : message.seed;
            else
                object.seed = options.longs === String ? $util.Long.prototype.toString.call(message.seed) : options.longs === Number ? new $util.LongBits(message.seed.low >>> 0, message.seed.high >>> 0).toNumber(true) : message.seed;
        if (message.reveals && message.reveals.length) {
            object.reveals = [];
            for (var j = 0; j < message.reveals.length; ++j)
                object.reveals[j] = $root.Reveal.toObject(message.reveals[j], options);
        }
        return object;
    };

    /**
     * Converts this ChunkSync to JSON.
     * @function toJSON
     * @memberof ChunkSync
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    ChunkSync.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for ChunkSync
     * @function getTypeUrl
     * @memberof ChunkSync
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    ChunkSync.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/ChunkSync";
    };

    return ChunkSync;
})();

$root.RevealAck = (function() {

    /**
     * Properties of a RevealAck.
     * @exports IRevealAck
     * @interface IRevealAck
     * @property {IChunkID|null} [chunkId] RevealAck chunkId
     * @property {number|null} [x] RevealAck x
     * @property {number|null} [y] RevealAck y
     * @property {boolean|null} [ok] RevealAck ok
     * @property {number|null} [scorer] RevealAck scorer
     */

    /**
     * Constructs a new RevealAck.
     * @exports RevealAck
     * @classdesc Represents a RevealAck.
     * @implements IRevealAck
     * @constructor
     * @param {IRevealAck=} [properties] Properties to set
     */
    function RevealAck(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * RevealAck chunkId.
     * @member {IChunkID|null|undefined} chunkId
     * @memberof RevealAck
     * @instance
     */
    RevealAck.prototype.chunkId = null;

    /**
     * RevealAck x.
     * @member {number} x
     * @memberof RevealAck
     * @instance
     */
    RevealAck.prototype.x = 0;

    /**
     * RevealAck y.
     * @member {number} y
     * @memberof RevealAck
     * @instance
     */
    RevealAck.prototype.y = 0;

    /**
     * RevealAck ok.
     * @member {boolean} ok
     * @memberof RevealAck
     * @instance
     */
    RevealAck.prototype.ok = false;

    /**
     * RevealAck scorer.
     * @member {number} scorer
     * @memberof RevealAck
     * @instance
     */
    RevealAck.prototype.scorer = 0;

    /**
     * Creates a new RevealAck instance using the specified properties.
     * @function create
     * @memberof RevealAck
     * @static
     * @param {IRevealAck=} [properties] Properties to set
     * @returns {RevealAck} RevealAck instance
     */
    RevealAck.create = function create(properties) {
        return new RevealAck(properties);
    };

    /**
     * Encodes the specified RevealAck message. Does not implicitly {@link RevealAck.verify|verify} messages.
     * @function encode
     * @memberof RevealAck
     * @static
     * @param {IRevealAck} message RevealAck message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    RevealAck.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
            $root.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.x != null && Object.hasOwnProperty.call(message, "x"))
            writer.uint32(/* id 2, wireType 0 =*/16).int32(message.x);
        if (message.y != null && Object.hasOwnProperty.call(message, "y"))
            writer.uint32(/* id 3, wireType 0 =*/24).int32(message.y);
        if (message.ok != null && Object.hasOwnProperty.call(message, "ok"))
            writer.uint32(/* id 4, wireType 0 =*/32).bool(message.ok);
        if (message.scorer != null && Object.hasOwnProperty.call(message, "scorer"))
            writer.uint32(/* id 5, wireType 0 =*/40).int32(message.scorer);
        return writer;
    };

    /**
     * Encodes the specified RevealAck message, length delimited. Does not implicitly {@link RevealAck.verify|verify} messages.
     * @function encodeDelimited
     * @memberof RevealAck
     * @static
     * @param {IRevealAck} message RevealAck message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    RevealAck.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a RevealAck message from the specified reader or buffer.
     * @function decode
     * @memberof RevealAck
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {RevealAck} RevealAck
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    RevealAck.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.RevealAck();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.chunkId = $root.ChunkID.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.x = reader.int32();
                    break;
                }
            case 3: {
                    message.y = reader.int32();
                    break;
                }
            case 4: {
                    message.ok = reader.bool();
                    break;
                }
            case 5: {
                    message.scorer = reader.int32();
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a RevealAck message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof RevealAck
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {RevealAck} RevealAck
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    RevealAck.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a RevealAck message.
     * @function verify
     * @memberof RevealAck
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    RevealAck.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
            var error = $root.ChunkID.verify(message.chunkId);
            if (error)
                return "chunkId." + error;
        }
        if (message.x != null && message.hasOwnProperty("x"))
            if (!$util.isInteger(message.x))
                return "x: integer expected";
        if (message.y != null && message.hasOwnProperty("y"))
            if (!$util.isInteger(message.y))
                return "y: integer expected";
        if (message.ok != null && message.hasOwnProperty("ok"))
            if (typeof message.ok !== "boolean")
                return "ok: boolean expected";
        if (message.scorer != null && message.hasOwnProperty("scorer"))
            if (!$util.isInteger(message.scorer))
                return "scorer: integer expected";
        return null;
    };

    /**
     * Creates a RevealAck message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof RevealAck
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {RevealAck} RevealAck
     */
    RevealAck.fromObject = function fromObject(object) {
        if (object instanceof $root.RevealAck)
            return object;
        var message = new $root.RevealAck();
        if (object.chunkId != null) {
            if (typeof object.chunkId !== "object")
                throw TypeError(".RevealAck.chunkId: object expected");
            message.chunkId = $root.ChunkID.fromObject(object.chunkId);
        }
        if (object.x != null)
            message.x = object.x | 0;
        if (object.y != null)
            message.y = object.y | 0;
        if (object.ok != null)
            message.ok = Boolean(object.ok);
        if (object.scorer != null)
            message.scorer = object.scorer | 0;
        return message;
    };

    /**
     * Creates a plain object from a RevealAck message. Also converts values to other types if specified.
     * @function toObject
     * @memberof RevealAck
     * @static
     * @param {RevealAck} message RevealAck
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    RevealAck.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.chunkId = null;
            object.x = 0;
            object.y = 0;
            object.ok = false;
            object.scorer = 0;
        }
        if (message.chunkId != null && message.hasOwnProperty("chunkId"))
            object.chunkId = $root.ChunkID.toObject(message.chunkId, options);
        if (message.x != null && message.hasOwnProperty("x"))
            object.x = message.x;
        if (message.y != null && message.hasOwnProperty("y"))
            object.y = message.y;
        if (message.ok != null && message.hasOwnProperty("ok"))
            object.ok = message.ok;
        if (message.scorer != null && message.hasOwnProperty("scorer"))
            object.scorer = message.scorer;
        return object;
    };

    /**
     * Converts this RevealAck to JSON.
     * @function toJSON
     * @memberof RevealAck
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    RevealAck.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for RevealAck
     * @function getTypeUrl
     * @memberof RevealAck
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    RevealAck.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/RevealAck";
    };

    return RevealAck;
})();

$root.LeaderboardEntry = (function() {

    /**
     * Properties of a LeaderboardEntry.
     * @exports ILeaderboardEntry
     * @interface ILeaderboardEntry
     * @property {number|null} [playerId] LeaderboardEntry playerId
     * @property {string|null} [score] LeaderboardEntry score
     */

    /**
     * Constructs a new LeaderboardEntry.
     * @exports LeaderboardEntry
     * @classdesc Represents a LeaderboardEntry.
     * @implements ILeaderboardEntry
     * @constructor
     * @param {ILeaderboardEntry=} [properties] Properties to set
     */
    function LeaderboardEntry(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * LeaderboardEntry playerId.
     * @member {number} playerId
     * @memberof LeaderboardEntry
     * @instance
     */
    LeaderboardEntry.prototype.playerId = 0;

    /**
     * LeaderboardEntry score.
     * @member {string} score
     * @memberof LeaderboardEntry
     * @instance
     */
    LeaderboardEntry.prototype.score = "";

    /**
     * Creates a new LeaderboardEntry instance using the specified properties.
     * @function create
     * @memberof LeaderboardEntry
     * @static
     * @param {ILeaderboardEntry=} [properties] Properties to set
     * @returns {LeaderboardEntry} LeaderboardEntry instance
     */
    LeaderboardEntry.create = function create(properties) {
        return new LeaderboardEntry(properties);
    };

    /**
     * Encodes the specified LeaderboardEntry message. Does not implicitly {@link LeaderboardEntry.verify|verify} messages.
     * @function encode
     * @memberof LeaderboardEntry
     * @static
     * @param {ILeaderboardEntry} message LeaderboardEntry message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    LeaderboardEntry.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
            writer.uint32(/* id 1, wireType 0 =*/8).int32(message.playerId);
        if (message.score != null && Object.hasOwnProperty.call(message, "score"))
            writer.uint32(/* id 2, wireType 2 =*/18).string(message.score);
        return writer;
    };

    /**
     * Encodes the specified LeaderboardEntry message, length delimited. Does not implicitly {@link LeaderboardEntry.verify|verify} messages.
     * @function encodeDelimited
     * @memberof LeaderboardEntry
     * @static
     * @param {ILeaderboardEntry} message LeaderboardEntry message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    LeaderboardEntry.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a LeaderboardEntry message from the specified reader or buffer.
     * @function decode
     * @memberof LeaderboardEntry
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {LeaderboardEntry} LeaderboardEntry
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    LeaderboardEntry.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.LeaderboardEntry();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.playerId = reader.int32();
                    break;
                }
            case 2: {
                    message.score = reader.string();
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a LeaderboardEntry message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof LeaderboardEntry
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {LeaderboardEntry} LeaderboardEntry
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    LeaderboardEntry.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a LeaderboardEntry message.
     * @function verify
     * @memberof LeaderboardEntry
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    LeaderboardEntry.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.playerId != null && message.hasOwnProperty("playerId"))
            if (!$util.isInteger(message.playerId))
                return "playerId: integer expected";
        if (message.score != null && message.hasOwnProperty("score"))
            if (!$util.isString(message.score))
                return "score: string expected";
        return null;
    };

    /**
     * Creates a LeaderboardEntry message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof LeaderboardEntry
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {LeaderboardEntry} LeaderboardEntry
     */
    LeaderboardEntry.fromObject = function fromObject(object) {
        if (object instanceof $root.LeaderboardEntry)
            return object;
        var message = new $root.LeaderboardEntry();
        if (object.playerId != null)
            message.playerId = object.playerId | 0;
        if (object.score != null)
            message.score = String(object.score);
        return message;
    };

    /**
     * Creates a plain object from a LeaderboardEntry message. Also converts values to other types if specified.
     * @function toObject
     * @memberof LeaderboardEntry
     * @static
     * @param {LeaderboardEntry} message LeaderboardEntry
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    LeaderboardEntry.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.playerId = 0;
            object.score = "";
        }
        if (message.playerId != null && message.hasOwnProperty("playerId"))
            object.playerId = message.playerId;
        if (message.score != null && message.hasOwnProperty("score"))
            object.score = message.score;
        return object;
    };

    /**
     * Converts this LeaderboardEntry to JSON.
     * @function toJSON
     * @memberof LeaderboardEntry
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    LeaderboardEntry.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for LeaderboardEntry
     * @function getTypeUrl
     * @memberof LeaderboardEntry
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    LeaderboardEntry.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/LeaderboardEntry";
    };

    return LeaderboardEntry;
})();

$root.Leaderboard = (function() {

    /**
     * Properties of a Leaderboard.
     * @exports ILeaderboard
     * @interface ILeaderboard
     * @property {number|Long|null} [version] Leaderboard version
     * @property {Array.<ILeaderboardEntry>|null} [entries] Leaderboard entries
     */

    /**
     * Constructs a new Leaderboard.
     * @exports Leaderboard
     * @classdesc Represents a Leaderboard.
     * @implements ILeaderboard
     * @constructor
     * @param {ILeaderboard=} [properties] Properties to set
     */
    function Leaderboard(properties) {
        this.entries = [];
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * Leaderboard version.
     * @member {number|Long} version
     * @memberof Leaderboard
     * @instance
     */
    Leaderboard.prototype.version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

    /**
     * Leaderboard entries.
     * @member {Array.<ILeaderboardEntry>} entries
     * @memberof Leaderboard
     * @instance
     */
    Leaderboard.prototype.entries = $util.emptyArray;

    /**
     * Creates a new Leaderboard instance using the specified properties.
     * @function create
     * @memberof Leaderboard
     * @static
     * @param {ILeaderboard=} [properties] Properties to set
     * @returns {Leaderboard} Leaderboard instance
     */
    Leaderboard.create = function create(properties) {
        return new Leaderboard(properties);
    };

    /**
     * Encodes the specified Leaderboard message. Does not implicitly {@link Leaderboard.verify|verify} messages.
     * @function encode
     * @memberof Leaderboard
     * @static
     * @param {ILeaderboard} message Leaderboard message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Leaderboard.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.version != null && Object.hasOwnProperty.call(message, "version"))
            writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.version);
        if (message.entries != null && message.entries.length)
            for (var i = 0; i < message.entries.length; ++i)
                $root.LeaderboardEntry.encode(message.entries[i], writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified Leaderboard message, length delimited. Does not implicitly {@link Leaderboard.verify|verify} messages.
     * @function encodeDelimited
     * @memberof Leaderboard
     * @static
     * @param {ILeaderboard} message Leaderboard message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Leaderboard.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a Leaderboard message from the specified reader or buffer.
     * @function decode
     * @memberof Leaderboard
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {Leaderboard} Leaderboard
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Leaderboard.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.Leaderboard();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.version = reader.uint64();
                    break;
                }
            case 2: {
                    if (!(message.entries && message.entries.length))
                        message.entries = [];
                    message.entries.push($root.LeaderboardEntry.decode(reader, reader.uint32()));
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a Leaderboard message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof Leaderboard
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {Leaderboard} Leaderboard
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Leaderboard.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a Leaderboard message.
     * @function verify
     * @memberof Leaderboard
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    Leaderboard.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.version != null && message.hasOwnProperty("version"))
            if (!$util.isInteger(message.version) && !(message.version && $util.isInteger(message.version.low) && $util.isInteger(message.version.high)))
                return "version: integer|Long expected";
        if (message.entries != null && message.hasOwnProperty("entries")) {
            if (!Array.isArray(message.entries))
                return "entries: array expected";
            for (var i = 0; i < message.entries.length; ++i) {
                var error = $root.LeaderboardEntry.verify(message.entries[i]);
                if (error)
                    return "entries." + error;
            }
        }
        return null;
    };

    /**
     * Creates a Leaderboard message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof Leaderboard
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {Leaderboard} Leaderboard
     */
    Leaderboard.fromObject = function fromObject(object) {
        if (object instanceof $root.Leaderboard)
            return object;
        var message = new $root.Leaderboard();
        if (object.version != null)
            if ($util.Long)
                (message.version = $util.Long.fromValue(object.version)).unsigned = true;
            else if (typeof object.version === "string")
                message.version = parseInt(object.version, 10);
            else if (typeof object.version === "number")
                message.version = object.version;
            else if (typeof object.version === "object")
                message.version = new $util.LongBits(object.version.low >>> 0, object.version.high >>> 0).toNumber(true);
        if (object.entries) {
            if (!Array.isArray(object.entries))
                throw TypeError(".Leaderboard.entries: array expected");
            message.entries = [];
            for (var i = 0; i < object.entries.length; ++i) {
                if (typeof object.entries[i] !== "object")
                    throw TypeError(".Leaderboard.entries: object expected");
                message.entries[i] = $root.LeaderboardEntry.fromObject(object.entries[i]);
            }
        }
        return message;
    };

    /**
     * Creates a plain object from a Leaderboard message. Also converts values to other types if specified.
     * @function toObject
     * @memberof Leaderboard
     * @static
     * @param {Leaderboard} message Leaderboard
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    Leaderboard.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.arrays || options.defaults)
            object.entries = [];
        if (options.defaults)
            if ($util.Long) {
                var long = new $util.Long(0, 0, true);
                object.version = options.longs === String ? long.toString() : options.longs === Number ? long.toNumber() : long;
            } else
                object.version = options.longs === String ? "0" : 0;
        if (message.version != null && message.hasOwnProperty("version"))
            if (typeof message.version === "number")
                object.version = options.longs === String ? String(message.version) : message.version;
            else
                object.version = options.longs === String ? $util.Long.prototype.toString.call(message.version) : options.longs === Number ? new $util.LongBits(message.version.low >>> 0, message.version.high >>> 0).toNumber(true) : message.version;
        if (message.entries && message.entries.length) {
            object.entries = [];
            for (var j = 0; j < message.entries.length; ++j)
                object.entries[j] = $root.LeaderboardEntry.toObject(message.entries[j], options);
        }
        return object;
    };

    /**
     * Converts this Leaderboard to JSON.
     * @function toJSON
     * @memberof Leaderboard
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    Leaderboard.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for Leaderboard
     * @function getTypeUrl
     * @memberof Leaderboard
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    Leaderboard.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/Leaderboard";
    };

    return Leaderboard;
})();

$root.ClientMessage = (function() {

    /**
     * Properties of a ClientMessage.
     * @exports IClientMessage
     * @interface IClientMessage
     * @property {IRevealRequest|null} [reveal] ClientMessage reveal
     * @property {ISubscribeRequest|null} [subscribe] ClientMessage subscribe
     * @property {IUnsubscribeRequest|null} [unsubscribe] ClientMessage unsubscribe
     * @property {ISeedRequest|null} [seed] ClientMessage seed
     */

    /**
     * Constructs a new ClientMessage.
     * @exports ClientMessage
     * @classdesc Represents a ClientMessage.
     * @implements IClientMessage
     * @constructor
     * @param {IClientMessage=} [properties] Properties to set
     */
    function ClientMessage(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * ClientMessage reveal.
     * @member {IRevealRequest|null|undefined} reveal
     * @memberof ClientMessage
     * @instance
     */
    ClientMessage.prototype.reveal = null;

    /**
     * ClientMessage subscribe.
     * @member {ISubscribeRequest|null|undefined} subscribe
     * @memberof ClientMessage
     * @instance
     */
    ClientMessage.prototype.subscribe = null;

    /**
     * ClientMessage unsubscribe.
     * @member {IUnsubscribeRequest|null|undefined} unsubscribe
     * @memberof ClientMessage
     * @instance
     */
    ClientMessage.prototype.unsubscribe = null;

    /**
     * ClientMessage seed.
     * @member {ISeedRequest|null|undefined} seed
     * @memberof ClientMessage
     * @instance
     */
    ClientMessage.prototype.seed = null;

    // OneOf field names bound to virtual getters and setters
    var $oneOfFields;

    /**
     * ClientMessage msg.
     * @member {"reveal"|"subscribe"|"unsubscribe"|"seed"|undefined} msg
     * @memberof ClientMessage
     * @instance
     */
    Object.defineProperty(ClientMessage.prototype, "msg", {
        get: $util.oneOfGetter($oneOfFields = ["reveal", "subscribe", "unsubscribe", "seed"]),
        set: $util.oneOfSetter($oneOfFields)
    });

    /**
     * Creates a new ClientMessage instance using the specified properties.
     * @function create
     * @memberof ClientMessage
     * @static
     * @param {IClientMessage=} [properties] Properties to set
     * @returns {ClientMessage} ClientMessage instance
     */
    ClientMessage.create = function create(properties) {
        return new ClientMessage(properties);
    };

    /**
     * Encodes the specified ClientMessage message. Does not implicitly {@link ClientMessage.verify|verify} messages.
     * @function encode
     * @memberof ClientMessage
     * @static
     * @param {IClientMessage} message ClientMessage message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ClientMessage.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.reveal != null && Object.hasOwnProperty.call(message, "reveal"))
            $root.RevealRequest.encode(message.reveal, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.subscribe != null && Object.hasOwnProperty.call(message, "subscribe"))
            $root.SubscribeRequest.encode(message.subscribe, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
        if (message.unsubscribe != null && Object.hasOwnProperty.call(message, "unsubscribe"))
            $root.UnsubscribeRequest.encode(message.unsubscribe, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
        if (message.seed != null && Object.hasOwnProperty.call(message, "seed"))
            $root.SeedRequest.encode(message.seed, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified ClientMessage message, length delimited. Does not implicitly {@link ClientMessage.verify|verify} messages.
     * @function encodeDelimited
     * @memberof ClientMessage
     * @static
     * @param {IClientMessage} message ClientMessage message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ClientMessage.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a ClientMessage message from the specified reader or buffer.
     * @function decode
     * @memberof ClientMessage
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {ClientMessage} ClientMessage
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ClientMessage.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.ClientMessage();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.reveal = $root.RevealRequest.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.subscribe = $root.SubscribeRequest.decode(reader, reader.uint32());
                    break;
                }
            case 3: {
                    message.unsubscribe = $root.UnsubscribeRequest.decode(reader, reader.uint32());
                    break;
                }
            case 4: {
                    message.seed = $root.SeedRequest.decode(reader, reader.uint32());
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a ClientMessage message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof ClientMessage
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {ClientMessage} ClientMessage
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ClientMessage.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a ClientMessage message.
     * @function verify
     * @memberof ClientMessage
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    ClientMessage.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        var properties = {};
        if (message.reveal != null && message.hasOwnProperty("reveal")) {
            properties.msg = 1;
            {
                var error = $root.RevealRequest.verify(message.reveal);
                if (error)
                    return "reveal." + error;
            }
        }
        if (message.subscribe != null && message.hasOwnProperty("subscribe")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.SubscribeRequest.verify(message.subscribe);
                if (error)
                    return "subscribe." + error;
            }
        }
        if (message.unsubscribe != null && message.hasOwnProperty("unsubscribe")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.UnsubscribeRequest.verify(message.unsubscribe);
                if (error)
                    return "unsubscribe." + error;
            }
        }
        if (message.seed != null && message.hasOwnProperty("seed")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.SeedRequest.verify(message.seed);
                if (error)
                    return "seed." + error;
            }
        }
        return null;
    };

    /**
     * Creates a ClientMessage message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof ClientMessage
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {ClientMessage} ClientMessage
     */
    ClientMessage.fromObject = function fromObject(object) {
        if (object instanceof $root.ClientMessage)
            return object;
        var message = new $root.ClientMessage();
        if (object.reveal != null) {
            if (typeof object.reveal !== "object")
                throw TypeError(".ClientMessage.reveal: object expected");
            message.reveal = $root.RevealRequest.fromObject(object.reveal);
        }
        if (object.subscribe != null) {
            if (typeof object.subscribe !== "object")
                throw TypeError(".ClientMessage.subscribe: object expected");
            message.subscribe = $root.SubscribeRequest.fromObject(object.subscribe);
        }
        if (object.unsubscribe != null) {
            if (typeof object.unsubscribe !== "object")
                throw TypeError(".ClientMessage.unsubscribe: object expected");
            message.unsubscribe = $root.UnsubscribeRequest.fromObject(object.unsubscribe);
        }
        if (object.seed != null) {
            if (typeof object.seed !== "object")
                throw TypeError(".ClientMessage.seed: object expected");
            message.seed = $root.SeedRequest.fromObject(object.seed);
        }
        return message;
    };

    /**
     * Creates a plain object from a ClientMessage message. Also converts values to other types if specified.
     * @function toObject
     * @memberof ClientMessage
     * @static
     * @param {ClientMessage} message ClientMessage
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    ClientMessage.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (message.reveal != null && message.hasOwnProperty("reveal")) {
            object.reveal = $root.RevealRequest.toObject(message.reveal, options);
            if (options.oneofs)
                object.msg = "reveal";
        }
        if (message.subscribe != null && message.hasOwnProperty("subscribe")) {
            object.subscribe = $root.SubscribeRequest.toObject(message.subscribe, options);
            if (options.oneofs)
                object.msg = "subscribe";
        }
        if (message.unsubscribe != null && message.hasOwnProperty("unsubscribe")) {
            object.unsubscribe = $root.UnsubscribeRequest.toObject(message.unsubscribe, options);
            if (options.oneofs)
                object.msg = "unsubscribe";
        }
        if (message.seed != null && message.hasOwnProperty("seed")) {
            object.seed = $root.SeedRequest.toObject(message.seed, options);
            if (options.oneofs)
                object.msg = "seed";
        }
        return object;
    };

    /**
     * Converts this ClientMessage to JSON.
     * @function toJSON
     * @memberof ClientMessage
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    ClientMessage.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for ClientMessage
     * @function getTypeUrl
     * @memberof ClientMessage
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    ClientMessage.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/ClientMessage";
    };

    return ClientMessage;
})();

$root.ServerMessage = (function() {

    /**
     * Properties of a ServerMessage.
     * @exports IServerMessage
     * @interface IServerMessage
     * @property {IReveal|null} [reveal] ServerMessage reveal
     * @property {IRevealAck|null} [revealAck] ServerMessage revealAck
     * @property {ISeedResponse|null} [seed] ServerMessage seed
     * @property {IChunkSync|null} [chunkSync] ServerMessage chunkSync
     * @property {ILeaderboard|null} [leaderboard] ServerMessage leaderboard
     */

    /**
     * Constructs a new ServerMessage.
     * @exports ServerMessage
     * @classdesc Represents a ServerMessage.
     * @implements IServerMessage
     * @constructor
     * @param {IServerMessage=} [properties] Properties to set
     */
    function ServerMessage(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * ServerMessage reveal.
     * @member {IReveal|null|undefined} reveal
     * @memberof ServerMessage
     * @instance
     */
    ServerMessage.prototype.reveal = null;

    /**
     * ServerMessage revealAck.
     * @member {IRevealAck|null|undefined} revealAck
     * @memberof ServerMessage
     * @instance
     */
    ServerMessage.prototype.revealAck = null;

    /**
     * ServerMessage seed.
     * @member {ISeedResponse|null|undefined} seed
     * @memberof ServerMessage
     * @instance
     */
    ServerMessage.prototype.seed = null;

    /**
     * ServerMessage chunkSync.
     * @member {IChunkSync|null|undefined} chunkSync
     * @memberof ServerMessage
     * @instance
     */
    ServerMessage.prototype.chunkSync = null;

    /**
     * ServerMessage leaderboard.
     * @member {ILeaderboard|null|undefined} leaderboard
     * @memberof ServerMessage
     * @instance
     */
    ServerMessage.prototype.leaderboard = null;

    // OneOf field names bound to virtual getters and setters
    var $oneOfFields;

    /**
     * ServerMessage msg.
     * @member {"reveal"|"revealAck"|"seed"|"chunkSync"|"leaderboard"|undefined} msg
     * @memberof ServerMessage
     * @instance
     */
    Object.defineProperty(ServerMessage.prototype, "msg", {
        get: $util.oneOfGetter($oneOfFields = ["reveal", "revealAck", "seed", "chunkSync", "leaderboard"]),
        set: $util.oneOfSetter($oneOfFields)
    });

    /**
     * Creates a new ServerMessage instance using the specified properties.
     * @function create
     * @memberof ServerMessage
     * @static
     * @param {IServerMessage=} [properties] Properties to set
     * @returns {ServerMessage} ServerMessage instance
     */
    ServerMessage.create = function create(properties) {
        return new ServerMessage(properties);
    };

    /**
     * Encodes the specified ServerMessage message. Does not implicitly {@link ServerMessage.verify|verify} messages.
     * @function encode
     * @memberof ServerMessage
     * @static
     * @param {IServerMessage} message ServerMessage message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ServerMessage.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.reveal != null && Object.hasOwnProperty.call(message, "reveal"))
            $root.Reveal.encode(message.reveal, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
        if (message.revealAck != null && Object.hasOwnProperty.call(message, "revealAck"))
            $root.RevealAck.encode(message.revealAck, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
        if (message.seed != null && Object.hasOwnProperty.call(message, "seed"))
            $root.SeedResponse.encode(message.seed, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
        if (message.chunkSync != null && Object.hasOwnProperty.call(message, "chunkSync"))
            $root.ChunkSync.encode(message.chunkSync, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
        if (message.leaderboard != null && Object.hasOwnProperty.call(message, "leaderboard"))
            $root.Leaderboard.encode(message.leaderboard, writer.uint32(/* id 5, wireType 2 =*/42).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified ServerMessage message, length delimited. Does not implicitly {@link ServerMessage.verify|verify} messages.
     * @function encodeDelimited
     * @memberof ServerMessage
     * @static
     * @param {IServerMessage} message ServerMessage message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    ServerMessage.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a ServerMessage message from the specified reader or buffer.
     * @function decode
     * @memberof ServerMessage
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {ServerMessage} ServerMessage
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ServerMessage.decode = function decode(reader, length, error) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.ServerMessage();
        while (reader.pos < end) {
            var tag = reader.uint32();
            if (tag === error)
                break;
            switch (tag >>> 3) {
            case 1: {
                    message.reveal = $root.Reveal.decode(reader, reader.uint32());
                    break;
                }
            case 2: {
                    message.revealAck = $root.RevealAck.decode(reader, reader.uint32());
                    break;
                }
            case 3: {
                    message.seed = $root.SeedResponse.decode(reader, reader.uint32());
                    break;
                }
            case 4: {
                    message.chunkSync = $root.ChunkSync.decode(reader, reader.uint32());
                    break;
                }
            case 5: {
                    message.leaderboard = $root.Leaderboard.decode(reader, reader.uint32());
                    break;
                }
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a ServerMessage message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof ServerMessage
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {ServerMessage} ServerMessage
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    ServerMessage.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a ServerMessage message.
     * @function verify
     * @memberof ServerMessage
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    ServerMessage.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        var properties = {};
        if (message.reveal != null && message.hasOwnProperty("reveal")) {
            properties.msg = 1;
            {
                var error = $root.Reveal.verify(message.reveal);
                if (error)
                    return "reveal." + error;
            }
        }
        if (message.revealAck != null && message.hasOwnProperty("revealAck")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.RevealAck.verify(message.revealAck);
                if (error)
                    return "revealAck." + error;
            }
        }
        if (message.seed != null && message.hasOwnProperty("seed")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.SeedResponse.verify(message.seed);
                if (error)
                    return "seed." + error;
            }
        }
        if (message.chunkSync != null && message.hasOwnProperty("chunkSync")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.ChunkSync.verify(message.chunkSync);
                if (error)
                    return "chunkSync." + error;
            }
        }
        if (message.leaderboard != null && message.hasOwnProperty("leaderboard")) {
            if (properties.msg === 1)
                return "msg: multiple values";
            properties.msg = 1;
            {
                var error = $root.Leaderboard.verify(message.leaderboard);
                if (error)
                    return "leaderboard." + error;
            }
        }
        return null;
    };

    /**
     * Creates a ServerMessage message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof ServerMessage
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {ServerMessage} ServerMessage
     */
    ServerMessage.fromObject = function fromObject(object) {
        if (object instanceof $root.ServerMessage)
            return object;
        var message = new $root.ServerMessage();
        if (object.reveal != null) {
            if (typeof object.reveal !== "object")
                throw TypeError(".ServerMessage.reveal: object expected");
            message.reveal = $root.Reveal.fromObject(object.reveal);
        }
        if (object.revealAck != null) {
            if (typeof object.revealAck !== "object")
                throw TypeError(".ServerMessage.revealAck: object expected");
            message.revealAck = $root.RevealAck.fromObject(object.revealAck);
        }
        if (object.seed != null) {
            if (typeof object.seed !== "object")
                throw TypeError(".ServerMessage.seed: object expected");
            message.seed = $root.SeedResponse.fromObject(object.seed);
        }
        if (object.chunkSync != null) {
            if (typeof object.chunkSync !== "object")
                throw TypeError(".ServerMessage.chunkSync: object expected");
            message.chunkSync = $root.ChunkSync.fromObject(object.chunkSync);
        }
        if (object.leaderboard != null) {
            if (typeof object.leaderboard !== "object")
                throw TypeError(".ServerMessage.leaderboard: object expected");
            message.leaderboard = $root.Leaderboard.fromObject(object.leaderboard);
        }
        return message;
    };

    /**
     * Creates a plain object from a ServerMessage message. Also converts values to other types if specified.
     * @function toObject
     * @memberof ServerMessage
     * @static
     * @param {ServerMessage} message ServerMessage
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    ServerMessage.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (message.reveal != null && message.hasOwnProperty("reveal")) {
            object.reveal = $root.Reveal.toObject(message.reveal, options);
            if (options.oneofs)
                object.msg = "reveal";
        }
        if (message.revealAck != null && message.hasOwnProperty("revealAck")) {
            object.revealAck = $root.RevealAck.toObject(message.revealAck, options);
            if (options.oneofs)
                object.msg = "revealAck";
        }
        if (message.seed != null && message.hasOwnProperty("seed")) {
            object.seed = $root.SeedResponse.toObject(message.seed, options);
            if (options.oneofs)
                object.msg = "seed";
        }
        if (message.chunkSync != null && message.hasOwnProperty("chunkSync")) {
            object.chunkSync = $root.ChunkSync.toObject(message.chunkSync, options);
            if (options.oneofs)
                object.msg = "chunkSync";
        }
        if (message.leaderboard != null && message.hasOwnProperty("leaderboard")) {
            object.leaderboard = $root.Leaderboard.toObject(message.leaderboard, options);
            if (options.oneofs)
                object.msg = "leaderboard";
        }
        return object;
    };

    /**
     * Converts this ServerMessage to JSON.
     * @function toJSON
     * @memberof ServerMessage
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    ServerMessage.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    /**
     * Gets the default type url for ServerMessage
     * @function getTypeUrl
     * @memberof ServerMessage
     * @static
     * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
     * @returns {string} The default type url
     */
    ServerMessage.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
        if (typeUrlPrefix === undefined) {
            typeUrlPrefix = "type.googleapis.com";
        }
        return typeUrlPrefix + "/ServerMessage";
    };

    return ServerMessage;
})();