/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
import * as $protobuf from "protobufjs/minimal";

// Common aliases
const $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
const $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

export const ms = $root.ms = (() => {

    /**
     * Namespace ms.
     * @exports ms
     * @namespace
     */
    const ms = {};

    ms.ChunkID = (function() {

        /**
         * Properties of a ChunkID.
         * @memberof ms
         * @interface IChunkID
         * @property {number|null} [X] ChunkID X
         * @property {number|null} [Y] ChunkID Y
         */

        /**
         * Constructs a new ChunkID.
         * @memberof ms
         * @classdesc Represents a ChunkID.
         * @implements IChunkID
         * @constructor
         * @param {ms.IChunkID=} [properties] Properties to set
         */
        function ChunkID(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ChunkID X.
         * @member {number} X
         * @memberof ms.ChunkID
         * @instance
         */
        ChunkID.prototype.X = 0;

        /**
         * ChunkID Y.
         * @member {number} Y
         * @memberof ms.ChunkID
         * @instance
         */
        ChunkID.prototype.Y = 0;

        /**
         * Creates a new ChunkID instance using the specified properties.
         * @function create
         * @memberof ms.ChunkID
         * @static
         * @param {ms.IChunkID=} [properties] Properties to set
         * @returns {ms.ChunkID} ChunkID instance
         */
        ChunkID.create = function create(properties) {
            return new ChunkID(properties);
        };

        /**
         * Encodes the specified ChunkID message. Does not implicitly {@link ms.ChunkID.verify|verify} messages.
         * @function encode
         * @memberof ms.ChunkID
         * @static
         * @param {ms.IChunkID} message ChunkID message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkID.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.X != null && Object.hasOwnProperty.call(message, "X"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.X);
            if (message.Y != null && Object.hasOwnProperty.call(message, "Y"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.Y);
            return writer;
        };

        /**
         * Encodes the specified ChunkID message, length delimited. Does not implicitly {@link ms.ChunkID.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.ChunkID
         * @static
         * @param {ms.IChunkID} message ChunkID message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkID.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ChunkID message from the specified reader or buffer.
         * @function decode
         * @memberof ms.ChunkID
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.ChunkID} ChunkID
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkID.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.ChunkID();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.X = reader.int32();
                        break;
                    }
                case 2: {
                        message.Y = reader.int32();
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
         * @memberof ms.ChunkID
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.ChunkID} ChunkID
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
         * @memberof ms.ChunkID
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ChunkID.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.X != null && message.hasOwnProperty("X"))
                if (!$util.isInteger(message.X))
                    return "X: integer expected";
            if (message.Y != null && message.hasOwnProperty("Y"))
                if (!$util.isInteger(message.Y))
                    return "Y: integer expected";
            return null;
        };

        /**
         * Creates a ChunkID message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.ChunkID
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.ChunkID} ChunkID
         */
        ChunkID.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.ChunkID)
                return object;
            let message = new $root.ms.ChunkID();
            if (object.X != null)
                message.X = object.X | 0;
            if (object.Y != null)
                message.Y = object.Y | 0;
            return message;
        };

        /**
         * Creates a plain object from a ChunkID message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.ChunkID
         * @static
         * @param {ms.ChunkID} message ChunkID
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ChunkID.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.X = 0;
                object.Y = 0;
            }
            if (message.X != null && message.hasOwnProperty("X"))
                object.X = message.X;
            if (message.Y != null && message.hasOwnProperty("Y"))
                object.Y = message.Y;
            return object;
        };

        /**
         * Converts this ChunkID to JSON.
         * @function toJSON
         * @memberof ms.ChunkID
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ChunkID.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ChunkID
         * @function getTypeUrl
         * @memberof ms.ChunkID
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ChunkID.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.ChunkID";
        };

        return ChunkID;
    })();

    ms.Reveal = (function() {

        /**
         * Properties of a Reveal.
         * @memberof ms
         * @interface IReveal
         * @property {ms.IChunkID|null} [chunkId] Reveal chunkId
         * @property {number|null} [x] Reveal x
         * @property {number|null} [y] Reveal y
         * @property {number|null} [playerId] Reveal playerId
         * @property {boolean|null} [flow] Reveal flow
         */

        /**
         * Constructs a new Reveal.
         * @memberof ms
         * @classdesc Represents a Reveal.
         * @implements IReveal
         * @constructor
         * @param {ms.IReveal=} [properties] Properties to set
         */
        function Reveal(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Reveal chunkId.
         * @member {ms.IChunkID|null|undefined} chunkId
         * @memberof ms.Reveal
         * @instance
         */
        Reveal.prototype.chunkId = null;

        /**
         * Reveal x.
         * @member {number} x
         * @memberof ms.Reveal
         * @instance
         */
        Reveal.prototype.x = 0;

        /**
         * Reveal y.
         * @member {number} y
         * @memberof ms.Reveal
         * @instance
         */
        Reveal.prototype.y = 0;

        /**
         * Reveal playerId.
         * @member {number} playerId
         * @memberof ms.Reveal
         * @instance
         */
        Reveal.prototype.playerId = 0;

        /**
         * Reveal flow.
         * @member {boolean} flow
         * @memberof ms.Reveal
         * @instance
         */
        Reveal.prototype.flow = false;

        /**
         * Creates a new Reveal instance using the specified properties.
         * @function create
         * @memberof ms.Reveal
         * @static
         * @param {ms.IReveal=} [properties] Properties to set
         * @returns {ms.Reveal} Reveal instance
         */
        Reveal.create = function create(properties) {
            return new Reveal(properties);
        };

        /**
         * Encodes the specified Reveal message. Does not implicitly {@link ms.Reveal.verify|verify} messages.
         * @function encode
         * @memberof ms.Reveal
         * @static
         * @param {ms.IReveal} message Reveal message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Reveal.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
                $root.ms.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.y);
            if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.playerId);
            if (message.flow != null && Object.hasOwnProperty.call(message, "flow"))
                writer.uint32(/* id 5, wireType 0 =*/40).bool(message.flow);
            return writer;
        };

        /**
         * Encodes the specified Reveal message, length delimited. Does not implicitly {@link ms.Reveal.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Reveal
         * @static
         * @param {ms.IReveal} message Reveal message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Reveal.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Reveal message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Reveal
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Reveal} Reveal
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Reveal.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Reveal();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunkId = $root.ms.ChunkID.decode(reader, reader.uint32());
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
                case 5: {
                        message.flow = reader.bool();
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
         * @memberof ms.Reveal
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Reveal} Reveal
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
         * @memberof ms.Reveal
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Reveal.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
                let error = $root.ms.ChunkID.verify(message.chunkId);
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
            if (message.flow != null && message.hasOwnProperty("flow"))
                if (typeof message.flow !== "boolean")
                    return "flow: boolean expected";
            return null;
        };

        /**
         * Creates a Reveal message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Reveal
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Reveal} Reveal
         */
        Reveal.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Reveal)
                return object;
            let message = new $root.ms.Reveal();
            if (object.chunkId != null) {
                if (typeof object.chunkId !== "object")
                    throw TypeError(".ms.Reveal.chunkId: object expected");
                message.chunkId = $root.ms.ChunkID.fromObject(object.chunkId);
            }
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            if (object.playerId != null)
                message.playerId = object.playerId | 0;
            if (object.flow != null)
                message.flow = Boolean(object.flow);
            return message;
        };

        /**
         * Creates a plain object from a Reveal message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Reveal
         * @static
         * @param {ms.Reveal} message Reveal
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Reveal.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.chunkId = null;
                object.x = 0;
                object.y = 0;
                object.playerId = 0;
                object.flow = false;
            }
            if (message.chunkId != null && message.hasOwnProperty("chunkId"))
                object.chunkId = $root.ms.ChunkID.toObject(message.chunkId, options);
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                object.playerId = message.playerId;
            if (message.flow != null && message.hasOwnProperty("flow"))
                object.flow = message.flow;
            return object;
        };

        /**
         * Converts this Reveal to JSON.
         * @function toJSON
         * @memberof ms.Reveal
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Reveal.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Reveal
         * @function getTypeUrl
         * @memberof ms.Reveal
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Reveal.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Reveal";
        };

        return Reveal;
    })();

    ms.Flag = (function() {

        /**
         * Properties of a Flag.
         * @memberof ms
         * @interface IFlag
         * @property {ms.IChunkID|null} [chunkId] Flag chunkId
         * @property {number|null} [x] Flag x
         * @property {number|null} [y] Flag y
         * @property {number|null} [playerId] Flag playerId
         * @property {string|null} [color] Flag color
         */

        /**
         * Constructs a new Flag.
         * @memberof ms
         * @classdesc Represents a Flag.
         * @implements IFlag
         * @constructor
         * @param {ms.IFlag=} [properties] Properties to set
         */
        function Flag(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Flag chunkId.
         * @member {ms.IChunkID|null|undefined} chunkId
         * @memberof ms.Flag
         * @instance
         */
        Flag.prototype.chunkId = null;

        /**
         * Flag x.
         * @member {number} x
         * @memberof ms.Flag
         * @instance
         */
        Flag.prototype.x = 0;

        /**
         * Flag y.
         * @member {number} y
         * @memberof ms.Flag
         * @instance
         */
        Flag.prototype.y = 0;

        /**
         * Flag playerId.
         * @member {number} playerId
         * @memberof ms.Flag
         * @instance
         */
        Flag.prototype.playerId = 0;

        /**
         * Flag color.
         * @member {string} color
         * @memberof ms.Flag
         * @instance
         */
        Flag.prototype.color = "";

        /**
         * Creates a new Flag instance using the specified properties.
         * @function create
         * @memberof ms.Flag
         * @static
         * @param {ms.IFlag=} [properties] Properties to set
         * @returns {ms.Flag} Flag instance
         */
        Flag.create = function create(properties) {
            return new Flag(properties);
        };

        /**
         * Encodes the specified Flag message. Does not implicitly {@link ms.Flag.verify|verify} messages.
         * @function encode
         * @memberof ms.Flag
         * @static
         * @param {ms.IFlag} message Flag message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Flag.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
                $root.ms.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.x != null && Object.hasOwnProperty.call(message, "x"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.x);
            if (message.y != null && Object.hasOwnProperty.call(message, "y"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.y);
            if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.playerId);
            if (message.color != null && Object.hasOwnProperty.call(message, "color"))
                writer.uint32(/* id 5, wireType 2 =*/42).string(message.color);
            return writer;
        };

        /**
         * Encodes the specified Flag message, length delimited. Does not implicitly {@link ms.Flag.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Flag
         * @static
         * @param {ms.IFlag} message Flag message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Flag.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Flag message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Flag
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Flag} Flag
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Flag.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Flag();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunkId = $root.ms.ChunkID.decode(reader, reader.uint32());
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
                case 5: {
                        message.color = reader.string();
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
         * Decodes a Flag message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.Flag
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Flag} Flag
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Flag.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Flag message.
         * @function verify
         * @memberof ms.Flag
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Flag.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
                let error = $root.ms.ChunkID.verify(message.chunkId);
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
            if (message.color != null && message.hasOwnProperty("color"))
                if (!$util.isString(message.color))
                    return "color: string expected";
            return null;
        };

        /**
         * Creates a Flag message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Flag
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Flag} Flag
         */
        Flag.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Flag)
                return object;
            let message = new $root.ms.Flag();
            if (object.chunkId != null) {
                if (typeof object.chunkId !== "object")
                    throw TypeError(".ms.Flag.chunkId: object expected");
                message.chunkId = $root.ms.ChunkID.fromObject(object.chunkId);
            }
            if (object.x != null)
                message.x = object.x | 0;
            if (object.y != null)
                message.y = object.y | 0;
            if (object.playerId != null)
                message.playerId = object.playerId | 0;
            if (object.color != null)
                message.color = String(object.color);
            return message;
        };

        /**
         * Creates a plain object from a Flag message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Flag
         * @static
         * @param {ms.Flag} message Flag
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Flag.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.chunkId = null;
                object.x = 0;
                object.y = 0;
                object.playerId = 0;
                object.color = "";
            }
            if (message.chunkId != null && message.hasOwnProperty("chunkId"))
                object.chunkId = $root.ms.ChunkID.toObject(message.chunkId, options);
            if (message.x != null && message.hasOwnProperty("x"))
                object.x = message.x;
            if (message.y != null && message.hasOwnProperty("y"))
                object.y = message.y;
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                object.playerId = message.playerId;
            if (message.color != null && message.hasOwnProperty("color"))
                object.color = message.color;
            return object;
        };

        /**
         * Converts this Flag to JSON.
         * @function toJSON
         * @memberof ms.Flag
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Flag.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Flag
         * @function getTypeUrl
         * @memberof ms.Flag
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Flag.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Flag";
        };

        return Flag;
    })();

    ms.Hello = (function() {

        /**
         * Properties of a Hello.
         * @memberof ms
         * @interface IHello
         * @property {number|null} [playerId] Hello playerId
         * @property {string|null} [name] Hello name
         * @property {string|null} [color] Hello color
         */

        /**
         * Constructs a new Hello.
         * @memberof ms
         * @classdesc Represents a Hello.
         * @implements IHello
         * @constructor
         * @param {ms.IHello=} [properties] Properties to set
         */
        function Hello(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Hello playerId.
         * @member {number} playerId
         * @memberof ms.Hello
         * @instance
         */
        Hello.prototype.playerId = 0;

        /**
         * Hello name.
         * @member {string} name
         * @memberof ms.Hello
         * @instance
         */
        Hello.prototype.name = "";

        /**
         * Hello color.
         * @member {string} color
         * @memberof ms.Hello
         * @instance
         */
        Hello.prototype.color = "";

        /**
         * Creates a new Hello instance using the specified properties.
         * @function create
         * @memberof ms.Hello
         * @static
         * @param {ms.IHello=} [properties] Properties to set
         * @returns {ms.Hello} Hello instance
         */
        Hello.create = function create(properties) {
            return new Hello(properties);
        };

        /**
         * Encodes the specified Hello message. Does not implicitly {@link ms.Hello.verify|verify} messages.
         * @function encode
         * @memberof ms.Hello
         * @static
         * @param {ms.IHello} message Hello message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Hello.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.playerId);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.color != null && Object.hasOwnProperty.call(message, "color"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.color);
            return writer;
        };

        /**
         * Encodes the specified Hello message, length delimited. Does not implicitly {@link ms.Hello.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Hello
         * @static
         * @param {ms.IHello} message Hello message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Hello.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Hello message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Hello
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Hello} Hello
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Hello.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Hello();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.playerId = reader.int32();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
                        message.color = reader.string();
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
         * Decodes a Hello message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.Hello
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Hello} Hello
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Hello.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Hello message.
         * @function verify
         * @memberof ms.Hello
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Hello.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                if (!$util.isInteger(message.playerId))
                    return "playerId: integer expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.color != null && message.hasOwnProperty("color"))
                if (!$util.isString(message.color))
                    return "color: string expected";
            return null;
        };

        /**
         * Creates a Hello message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Hello
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Hello} Hello
         */
        Hello.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Hello)
                return object;
            let message = new $root.ms.Hello();
            if (object.playerId != null)
                message.playerId = object.playerId | 0;
            if (object.name != null)
                message.name = String(object.name);
            if (object.color != null)
                message.color = String(object.color);
            return message;
        };

        /**
         * Creates a plain object from a Hello message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Hello
         * @static
         * @param {ms.Hello} message Hello
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Hello.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.playerId = 0;
                object.name = "";
                object.color = "";
            }
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                object.playerId = message.playerId;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.color != null && message.hasOwnProperty("color"))
                object.color = message.color;
            return object;
        };

        /**
         * Converts this Hello to JSON.
         * @function toJSON
         * @memberof ms.Hello
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Hello.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Hello
         * @function getTypeUrl
         * @memberof ms.Hello
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Hello.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Hello";
        };

        return Hello;
    })();

    ms.Welcome = (function() {

        /**
         * Properties of a Welcome.
         * @memberof ms
         * @interface IWelcome
         * @property {number|null} [playerId] Welcome playerId
         * @property {string|null} [name] Welcome name
         * @property {string|null} [color] Welcome color
         * @property {number|null} [viewX] Welcome viewX
         * @property {number|null} [viewY] Welcome viewY
         */

        /**
         * Constructs a new Welcome.
         * @memberof ms
         * @classdesc Represents a Welcome.
         * @implements IWelcome
         * @constructor
         * @param {ms.IWelcome=} [properties] Properties to set
         */
        function Welcome(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Welcome playerId.
         * @member {number} playerId
         * @memberof ms.Welcome
         * @instance
         */
        Welcome.prototype.playerId = 0;

        /**
         * Welcome name.
         * @member {string} name
         * @memberof ms.Welcome
         * @instance
         */
        Welcome.prototype.name = "";

        /**
         * Welcome color.
         * @member {string} color
         * @memberof ms.Welcome
         * @instance
         */
        Welcome.prototype.color = "";

        /**
         * Welcome viewX.
         * @member {number} viewX
         * @memberof ms.Welcome
         * @instance
         */
        Welcome.prototype.viewX = 0;

        /**
         * Welcome viewY.
         * @member {number} viewY
         * @memberof ms.Welcome
         * @instance
         */
        Welcome.prototype.viewY = 0;

        /**
         * Creates a new Welcome instance using the specified properties.
         * @function create
         * @memberof ms.Welcome
         * @static
         * @param {ms.IWelcome=} [properties] Properties to set
         * @returns {ms.Welcome} Welcome instance
         */
        Welcome.create = function create(properties) {
            return new Welcome(properties);
        };

        /**
         * Encodes the specified Welcome message. Does not implicitly {@link ms.Welcome.verify|verify} messages.
         * @function encode
         * @memberof ms.Welcome
         * @static
         * @param {ms.IWelcome} message Welcome message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Welcome.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.playerId);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.color != null && Object.hasOwnProperty.call(message, "color"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.color);
            if (message.viewX != null && Object.hasOwnProperty.call(message, "viewX"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.viewX);
            if (message.viewY != null && Object.hasOwnProperty.call(message, "viewY"))
                writer.uint32(/* id 5, wireType 0 =*/40).int32(message.viewY);
            return writer;
        };

        /**
         * Encodes the specified Welcome message, length delimited. Does not implicitly {@link ms.Welcome.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Welcome
         * @static
         * @param {ms.IWelcome} message Welcome message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Welcome.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Welcome message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Welcome
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Welcome} Welcome
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Welcome.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Welcome();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.playerId = reader.int32();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
                        message.color = reader.string();
                        break;
                    }
                case 4: {
                        message.viewX = reader.int32();
                        break;
                    }
                case 5: {
                        message.viewY = reader.int32();
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
         * Decodes a Welcome message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.Welcome
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Welcome} Welcome
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Welcome.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Welcome message.
         * @function verify
         * @memberof ms.Welcome
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Welcome.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                if (!$util.isInteger(message.playerId))
                    return "playerId: integer expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.color != null && message.hasOwnProperty("color"))
                if (!$util.isString(message.color))
                    return "color: string expected";
            if (message.viewX != null && message.hasOwnProperty("viewX"))
                if (!$util.isInteger(message.viewX))
                    return "viewX: integer expected";
            if (message.viewY != null && message.hasOwnProperty("viewY"))
                if (!$util.isInteger(message.viewY))
                    return "viewY: integer expected";
            return null;
        };

        /**
         * Creates a Welcome message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Welcome
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Welcome} Welcome
         */
        Welcome.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Welcome)
                return object;
            let message = new $root.ms.Welcome();
            if (object.playerId != null)
                message.playerId = object.playerId | 0;
            if (object.name != null)
                message.name = String(object.name);
            if (object.color != null)
                message.color = String(object.color);
            if (object.viewX != null)
                message.viewX = object.viewX | 0;
            if (object.viewY != null)
                message.viewY = object.viewY | 0;
            return message;
        };

        /**
         * Creates a plain object from a Welcome message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Welcome
         * @static
         * @param {ms.Welcome} message Welcome
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Welcome.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.playerId = 0;
                object.name = "";
                object.color = "";
                object.viewX = 0;
                object.viewY = 0;
            }
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                object.playerId = message.playerId;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.color != null && message.hasOwnProperty("color"))
                object.color = message.color;
            if (message.viewX != null && message.hasOwnProperty("viewX"))
                object.viewX = message.viewX;
            if (message.viewY != null && message.hasOwnProperty("viewY"))
                object.viewY = message.viewY;
            return object;
        };

        /**
         * Converts this Welcome to JSON.
         * @function toJSON
         * @memberof ms.Welcome
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Welcome.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Welcome
         * @function getTypeUrl
         * @memberof ms.Welcome
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Welcome.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Welcome";
        };

        return Welcome;
    })();

    ms.Subscribe = (function() {

        /**
         * Properties of a Subscribe.
         * @memberof ms
         * @interface ISubscribe
         * @property {number|null} [chunkX] Subscribe chunkX
         * @property {number|null} [chunkY] Subscribe chunkY
         */

        /**
         * Constructs a new Subscribe.
         * @memberof ms
         * @classdesc Represents a Subscribe.
         * @implements ISubscribe
         * @constructor
         * @param {ms.ISubscribe=} [properties] Properties to set
         */
        function Subscribe(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Subscribe chunkX.
         * @member {number} chunkX
         * @memberof ms.Subscribe
         * @instance
         */
        Subscribe.prototype.chunkX = 0;

        /**
         * Subscribe chunkY.
         * @member {number} chunkY
         * @memberof ms.Subscribe
         * @instance
         */
        Subscribe.prototype.chunkY = 0;

        /**
         * Creates a new Subscribe instance using the specified properties.
         * @function create
         * @memberof ms.Subscribe
         * @static
         * @param {ms.ISubscribe=} [properties] Properties to set
         * @returns {ms.Subscribe} Subscribe instance
         */
        Subscribe.create = function create(properties) {
            return new Subscribe(properties);
        };

        /**
         * Encodes the specified Subscribe message. Does not implicitly {@link ms.Subscribe.verify|verify} messages.
         * @function encode
         * @memberof ms.Subscribe
         * @static
         * @param {ms.ISubscribe} message Subscribe message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Subscribe.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunkX != null && Object.hasOwnProperty.call(message, "chunkX"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.chunkX);
            if (message.chunkY != null && Object.hasOwnProperty.call(message, "chunkY"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.chunkY);
            return writer;
        };

        /**
         * Encodes the specified Subscribe message, length delimited. Does not implicitly {@link ms.Subscribe.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Subscribe
         * @static
         * @param {ms.ISubscribe} message Subscribe message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Subscribe.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Subscribe message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Subscribe
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Subscribe} Subscribe
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Subscribe.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Subscribe();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunkX = reader.int32();
                        break;
                    }
                case 2: {
                        message.chunkY = reader.int32();
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
         * Decodes a Subscribe message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.Subscribe
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Subscribe} Subscribe
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Subscribe.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Subscribe message.
         * @function verify
         * @memberof ms.Subscribe
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Subscribe.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunkX != null && message.hasOwnProperty("chunkX"))
                if (!$util.isInteger(message.chunkX))
                    return "chunkX: integer expected";
            if (message.chunkY != null && message.hasOwnProperty("chunkY"))
                if (!$util.isInteger(message.chunkY))
                    return "chunkY: integer expected";
            return null;
        };

        /**
         * Creates a Subscribe message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Subscribe
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Subscribe} Subscribe
         */
        Subscribe.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Subscribe)
                return object;
            let message = new $root.ms.Subscribe();
            if (object.chunkX != null)
                message.chunkX = object.chunkX | 0;
            if (object.chunkY != null)
                message.chunkY = object.chunkY | 0;
            return message;
        };

        /**
         * Creates a plain object from a Subscribe message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Subscribe
         * @static
         * @param {ms.Subscribe} message Subscribe
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Subscribe.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.chunkX = 0;
                object.chunkY = 0;
            }
            if (message.chunkX != null && message.hasOwnProperty("chunkX"))
                object.chunkX = message.chunkX;
            if (message.chunkY != null && message.hasOwnProperty("chunkY"))
                object.chunkY = message.chunkY;
            return object;
        };

        /**
         * Converts this Subscribe to JSON.
         * @function toJSON
         * @memberof ms.Subscribe
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Subscribe.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Subscribe
         * @function getTypeUrl
         * @memberof ms.Subscribe
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Subscribe.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Subscribe";
        };

        return Subscribe;
    })();

    ms.Unsubscribe = (function() {

        /**
         * Properties of an Unsubscribe.
         * @memberof ms
         * @interface IUnsubscribe
         * @property {number|null} [chunkX] Unsubscribe chunkX
         * @property {number|null} [chunkY] Unsubscribe chunkY
         */

        /**
         * Constructs a new Unsubscribe.
         * @memberof ms
         * @classdesc Represents an Unsubscribe.
         * @implements IUnsubscribe
         * @constructor
         * @param {ms.IUnsubscribe=} [properties] Properties to set
         */
        function Unsubscribe(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Unsubscribe chunkX.
         * @member {number} chunkX
         * @memberof ms.Unsubscribe
         * @instance
         */
        Unsubscribe.prototype.chunkX = 0;

        /**
         * Unsubscribe chunkY.
         * @member {number} chunkY
         * @memberof ms.Unsubscribe
         * @instance
         */
        Unsubscribe.prototype.chunkY = 0;

        /**
         * Creates a new Unsubscribe instance using the specified properties.
         * @function create
         * @memberof ms.Unsubscribe
         * @static
         * @param {ms.IUnsubscribe=} [properties] Properties to set
         * @returns {ms.Unsubscribe} Unsubscribe instance
         */
        Unsubscribe.create = function create(properties) {
            return new Unsubscribe(properties);
        };

        /**
         * Encodes the specified Unsubscribe message. Does not implicitly {@link ms.Unsubscribe.verify|verify} messages.
         * @function encode
         * @memberof ms.Unsubscribe
         * @static
         * @param {ms.IUnsubscribe} message Unsubscribe message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Unsubscribe.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunkX != null && Object.hasOwnProperty.call(message, "chunkX"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.chunkX);
            if (message.chunkY != null && Object.hasOwnProperty.call(message, "chunkY"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.chunkY);
            return writer;
        };

        /**
         * Encodes the specified Unsubscribe message, length delimited. Does not implicitly {@link ms.Unsubscribe.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Unsubscribe
         * @static
         * @param {ms.IUnsubscribe} message Unsubscribe message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Unsubscribe.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an Unsubscribe message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Unsubscribe
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Unsubscribe} Unsubscribe
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Unsubscribe.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Unsubscribe();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunkX = reader.int32();
                        break;
                    }
                case 2: {
                        message.chunkY = reader.int32();
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
         * Decodes an Unsubscribe message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.Unsubscribe
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Unsubscribe} Unsubscribe
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Unsubscribe.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an Unsubscribe message.
         * @function verify
         * @memberof ms.Unsubscribe
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Unsubscribe.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunkX != null && message.hasOwnProperty("chunkX"))
                if (!$util.isInteger(message.chunkX))
                    return "chunkX: integer expected";
            if (message.chunkY != null && message.hasOwnProperty("chunkY"))
                if (!$util.isInteger(message.chunkY))
                    return "chunkY: integer expected";
            return null;
        };

        /**
         * Creates an Unsubscribe message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Unsubscribe
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Unsubscribe} Unsubscribe
         */
        Unsubscribe.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Unsubscribe)
                return object;
            let message = new $root.ms.Unsubscribe();
            if (object.chunkX != null)
                message.chunkX = object.chunkX | 0;
            if (object.chunkY != null)
                message.chunkY = object.chunkY | 0;
            return message;
        };

        /**
         * Creates a plain object from an Unsubscribe message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Unsubscribe
         * @static
         * @param {ms.Unsubscribe} message Unsubscribe
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Unsubscribe.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.chunkX = 0;
                object.chunkY = 0;
            }
            if (message.chunkX != null && message.hasOwnProperty("chunkX"))
                object.chunkX = message.chunkX;
            if (message.chunkY != null && message.hasOwnProperty("chunkY"))
                object.chunkY = message.chunkY;
            return object;
        };

        /**
         * Converts this Unsubscribe to JSON.
         * @function toJSON
         * @memberof ms.Unsubscribe
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Unsubscribe.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Unsubscribe
         * @function getTypeUrl
         * @memberof ms.Unsubscribe
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Unsubscribe.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Unsubscribe";
        };

        return Unsubscribe;
    })();

    ms.ChunkSync = (function() {

        /**
         * Properties of a ChunkSync.
         * @memberof ms
         * @interface IChunkSync
         * @property {ms.IChunkID|null} [chunkId] ChunkSync chunkId
         * @property {Uint8Array|null} [seed] ChunkSync seed
         * @property {Array.<ms.IReveal>|null} [reveals] ChunkSync reveals
         * @property {Array.<ms.IFlag>|null} [flags] ChunkSync flags
         */

        /**
         * Constructs a new ChunkSync.
         * @memberof ms
         * @classdesc Represents a ChunkSync.
         * @implements IChunkSync
         * @constructor
         * @param {ms.IChunkSync=} [properties] Properties to set
         */
        function ChunkSync(properties) {
            this.reveals = [];
            this.flags = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ChunkSync chunkId.
         * @member {ms.IChunkID|null|undefined} chunkId
         * @memberof ms.ChunkSync
         * @instance
         */
        ChunkSync.prototype.chunkId = null;

        /**
         * ChunkSync seed.
         * @member {Uint8Array} seed
         * @memberof ms.ChunkSync
         * @instance
         */
        ChunkSync.prototype.seed = $util.newBuffer([]);

        /**
         * ChunkSync reveals.
         * @member {Array.<ms.IReveal>} reveals
         * @memberof ms.ChunkSync
         * @instance
         */
        ChunkSync.prototype.reveals = $util.emptyArray;

        /**
         * ChunkSync flags.
         * @member {Array.<ms.IFlag>} flags
         * @memberof ms.ChunkSync
         * @instance
         */
        ChunkSync.prototype.flags = $util.emptyArray;

        /**
         * Creates a new ChunkSync instance using the specified properties.
         * @function create
         * @memberof ms.ChunkSync
         * @static
         * @param {ms.IChunkSync=} [properties] Properties to set
         * @returns {ms.ChunkSync} ChunkSync instance
         */
        ChunkSync.create = function create(properties) {
            return new ChunkSync(properties);
        };

        /**
         * Encodes the specified ChunkSync message. Does not implicitly {@link ms.ChunkSync.verify|verify} messages.
         * @function encode
         * @memberof ms.ChunkSync
         * @static
         * @param {ms.IChunkSync} message ChunkSync message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkSync.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.chunkId != null && Object.hasOwnProperty.call(message, "chunkId"))
                $root.ms.ChunkID.encode(message.chunkId, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.seed != null && Object.hasOwnProperty.call(message, "seed"))
                writer.uint32(/* id 2, wireType 2 =*/18).bytes(message.seed);
            if (message.reveals != null && message.reveals.length)
                for (let i = 0; i < message.reveals.length; ++i)
                    $root.ms.Reveal.encode(message.reveals[i], writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            if (message.flags != null && message.flags.length)
                for (let i = 0; i < message.flags.length; ++i)
                    $root.ms.Flag.encode(message.flags[i], writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ChunkSync message, length delimited. Does not implicitly {@link ms.ChunkSync.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.ChunkSync
         * @static
         * @param {ms.IChunkSync} message ChunkSync message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ChunkSync.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ChunkSync message from the specified reader or buffer.
         * @function decode
         * @memberof ms.ChunkSync
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.ChunkSync} ChunkSync
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ChunkSync.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.ChunkSync();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.chunkId = $root.ms.ChunkID.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.seed = reader.bytes();
                        break;
                    }
                case 3: {
                        if (!(message.reveals && message.reveals.length))
                            message.reveals = [];
                        message.reveals.push($root.ms.Reveal.decode(reader, reader.uint32()));
                        break;
                    }
                case 4: {
                        if (!(message.flags && message.flags.length))
                            message.flags = [];
                        message.flags.push($root.ms.Flag.decode(reader, reader.uint32()));
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
         * @memberof ms.ChunkSync
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.ChunkSync} ChunkSync
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
         * @memberof ms.ChunkSync
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ChunkSync.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.chunkId != null && message.hasOwnProperty("chunkId")) {
                let error = $root.ms.ChunkID.verify(message.chunkId);
                if (error)
                    return "chunkId." + error;
            }
            if (message.seed != null && message.hasOwnProperty("seed"))
                if (!(message.seed && typeof message.seed.length === "number" || $util.isString(message.seed)))
                    return "seed: buffer expected";
            if (message.reveals != null && message.hasOwnProperty("reveals")) {
                if (!Array.isArray(message.reveals))
                    return "reveals: array expected";
                for (let i = 0; i < message.reveals.length; ++i) {
                    let error = $root.ms.Reveal.verify(message.reveals[i]);
                    if (error)
                        return "reveals." + error;
                }
            }
            if (message.flags != null && message.hasOwnProperty("flags")) {
                if (!Array.isArray(message.flags))
                    return "flags: array expected";
                for (let i = 0; i < message.flags.length; ++i) {
                    let error = $root.ms.Flag.verify(message.flags[i]);
                    if (error)
                        return "flags." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ChunkSync message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.ChunkSync
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.ChunkSync} ChunkSync
         */
        ChunkSync.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.ChunkSync)
                return object;
            let message = new $root.ms.ChunkSync();
            if (object.chunkId != null) {
                if (typeof object.chunkId !== "object")
                    throw TypeError(".ms.ChunkSync.chunkId: object expected");
                message.chunkId = $root.ms.ChunkID.fromObject(object.chunkId);
            }
            if (object.seed != null)
                if (typeof object.seed === "string")
                    $util.base64.decode(object.seed, message.seed = $util.newBuffer($util.base64.length(object.seed)), 0);
                else if (object.seed.length >= 0)
                    message.seed = object.seed;
            if (object.reveals) {
                if (!Array.isArray(object.reveals))
                    throw TypeError(".ms.ChunkSync.reveals: array expected");
                message.reveals = [];
                for (let i = 0; i < object.reveals.length; ++i) {
                    if (typeof object.reveals[i] !== "object")
                        throw TypeError(".ms.ChunkSync.reveals: object expected");
                    message.reveals[i] = $root.ms.Reveal.fromObject(object.reveals[i]);
                }
            }
            if (object.flags) {
                if (!Array.isArray(object.flags))
                    throw TypeError(".ms.ChunkSync.flags: array expected");
                message.flags = [];
                for (let i = 0; i < object.flags.length; ++i) {
                    if (typeof object.flags[i] !== "object")
                        throw TypeError(".ms.ChunkSync.flags: object expected");
                    message.flags[i] = $root.ms.Flag.fromObject(object.flags[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a ChunkSync message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.ChunkSync
         * @static
         * @param {ms.ChunkSync} message ChunkSync
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ChunkSync.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults) {
                object.reveals = [];
                object.flags = [];
            }
            if (options.defaults) {
                object.chunkId = null;
                if (options.bytes === String)
                    object.seed = "";
                else {
                    object.seed = [];
                    if (options.bytes !== Array)
                        object.seed = $util.newBuffer(object.seed);
                }
            }
            if (message.chunkId != null && message.hasOwnProperty("chunkId"))
                object.chunkId = $root.ms.ChunkID.toObject(message.chunkId, options);
            if (message.seed != null && message.hasOwnProperty("seed"))
                object.seed = options.bytes === String ? $util.base64.encode(message.seed, 0, message.seed.length) : options.bytes === Array ? Array.prototype.slice.call(message.seed) : message.seed;
            if (message.reveals && message.reveals.length) {
                object.reveals = [];
                for (let j = 0; j < message.reveals.length; ++j)
                    object.reveals[j] = $root.ms.Reveal.toObject(message.reveals[j], options);
            }
            if (message.flags && message.flags.length) {
                object.flags = [];
                for (let j = 0; j < message.flags.length; ++j)
                    object.flags[j] = $root.ms.Flag.toObject(message.flags[j], options);
            }
            return object;
        };

        /**
         * Converts this ChunkSync to JSON.
         * @function toJSON
         * @memberof ms.ChunkSync
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ChunkSync.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ChunkSync
         * @function getTypeUrl
         * @memberof ms.ChunkSync
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ChunkSync.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.ChunkSync";
        };

        return ChunkSync;
    })();

    ms.RevealAck = (function() {

        /**
         * Properties of a RevealAck.
         * @memberof ms
         * @interface IRevealAck
         * @property {boolean|null} [ok] RevealAck ok
         */

        /**
         * Constructs a new RevealAck.
         * @memberof ms
         * @classdesc Represents a RevealAck.
         * @implements IRevealAck
         * @constructor
         * @param {ms.IRevealAck=} [properties] Properties to set
         */
        function RevealAck(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * RevealAck ok.
         * @member {boolean} ok
         * @memberof ms.RevealAck
         * @instance
         */
        RevealAck.prototype.ok = false;

        /**
         * Creates a new RevealAck instance using the specified properties.
         * @function create
         * @memberof ms.RevealAck
         * @static
         * @param {ms.IRevealAck=} [properties] Properties to set
         * @returns {ms.RevealAck} RevealAck instance
         */
        RevealAck.create = function create(properties) {
            return new RevealAck(properties);
        };

        /**
         * Encodes the specified RevealAck message. Does not implicitly {@link ms.RevealAck.verify|verify} messages.
         * @function encode
         * @memberof ms.RevealAck
         * @static
         * @param {ms.IRevealAck} message RevealAck message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RevealAck.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.ok != null && Object.hasOwnProperty.call(message, "ok"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.ok);
            return writer;
        };

        /**
         * Encodes the specified RevealAck message, length delimited. Does not implicitly {@link ms.RevealAck.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.RevealAck
         * @static
         * @param {ms.IRevealAck} message RevealAck message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RevealAck.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a RevealAck message from the specified reader or buffer.
         * @function decode
         * @memberof ms.RevealAck
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.RevealAck} RevealAck
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RevealAck.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.RevealAck();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.ok = reader.bool();
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
         * @memberof ms.RevealAck
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.RevealAck} RevealAck
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
         * @memberof ms.RevealAck
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        RevealAck.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.ok != null && message.hasOwnProperty("ok"))
                if (typeof message.ok !== "boolean")
                    return "ok: boolean expected";
            return null;
        };

        /**
         * Creates a RevealAck message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.RevealAck
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.RevealAck} RevealAck
         */
        RevealAck.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.RevealAck)
                return object;
            let message = new $root.ms.RevealAck();
            if (object.ok != null)
                message.ok = Boolean(object.ok);
            return message;
        };

        /**
         * Creates a plain object from a RevealAck message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.RevealAck
         * @static
         * @param {ms.RevealAck} message RevealAck
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        RevealAck.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.ok = false;
            if (message.ok != null && message.hasOwnProperty("ok"))
                object.ok = message.ok;
            return object;
        };

        /**
         * Converts this RevealAck to JSON.
         * @function toJSON
         * @memberof ms.RevealAck
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        RevealAck.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for RevealAck
         * @function getTypeUrl
         * @memberof ms.RevealAck
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        RevealAck.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.RevealAck";
        };

        return RevealAck;
    })();

    ms.FlagAck = (function() {

        /**
         * Properties of a FlagAck.
         * @memberof ms
         * @interface IFlagAck
         * @property {boolean|null} [ok] FlagAck ok
         */

        /**
         * Constructs a new FlagAck.
         * @memberof ms
         * @classdesc Represents a FlagAck.
         * @implements IFlagAck
         * @constructor
         * @param {ms.IFlagAck=} [properties] Properties to set
         */
        function FlagAck(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * FlagAck ok.
         * @member {boolean} ok
         * @memberof ms.FlagAck
         * @instance
         */
        FlagAck.prototype.ok = false;

        /**
         * Creates a new FlagAck instance using the specified properties.
         * @function create
         * @memberof ms.FlagAck
         * @static
         * @param {ms.IFlagAck=} [properties] Properties to set
         * @returns {ms.FlagAck} FlagAck instance
         */
        FlagAck.create = function create(properties) {
            return new FlagAck(properties);
        };

        /**
         * Encodes the specified FlagAck message. Does not implicitly {@link ms.FlagAck.verify|verify} messages.
         * @function encode
         * @memberof ms.FlagAck
         * @static
         * @param {ms.IFlagAck} message FlagAck message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        FlagAck.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.ok != null && Object.hasOwnProperty.call(message, "ok"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.ok);
            return writer;
        };

        /**
         * Encodes the specified FlagAck message, length delimited. Does not implicitly {@link ms.FlagAck.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.FlagAck
         * @static
         * @param {ms.IFlagAck} message FlagAck message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        FlagAck.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a FlagAck message from the specified reader or buffer.
         * @function decode
         * @memberof ms.FlagAck
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.FlagAck} FlagAck
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        FlagAck.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.FlagAck();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.ok = reader.bool();
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
         * Decodes a FlagAck message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.FlagAck
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.FlagAck} FlagAck
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        FlagAck.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a FlagAck message.
         * @function verify
         * @memberof ms.FlagAck
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        FlagAck.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.ok != null && message.hasOwnProperty("ok"))
                if (typeof message.ok !== "boolean")
                    return "ok: boolean expected";
            return null;
        };

        /**
         * Creates a FlagAck message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.FlagAck
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.FlagAck} FlagAck
         */
        FlagAck.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.FlagAck)
                return object;
            let message = new $root.ms.FlagAck();
            if (object.ok != null)
                message.ok = Boolean(object.ok);
            return message;
        };

        /**
         * Creates a plain object from a FlagAck message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.FlagAck
         * @static
         * @param {ms.FlagAck} message FlagAck
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        FlagAck.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.ok = false;
            if (message.ok != null && message.hasOwnProperty("ok"))
                object.ok = message.ok;
            return object;
        };

        /**
         * Converts this FlagAck to JSON.
         * @function toJSON
         * @memberof ms.FlagAck
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        FlagAck.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for FlagAck
         * @function getTypeUrl
         * @memberof ms.FlagAck
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        FlagAck.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.FlagAck";
        };

        return FlagAck;
    })();

    ms.LeaderboardEntry = (function() {

        /**
         * Properties of a LeaderboardEntry.
         * @memberof ms
         * @interface ILeaderboardEntry
         * @property {number|null} [playerId] LeaderboardEntry playerId
         * @property {string|null} [name] LeaderboardEntry name
         * @property {string|null} [score] LeaderboardEntry score
         */

        /**
         * Constructs a new LeaderboardEntry.
         * @memberof ms
         * @classdesc Represents a LeaderboardEntry.
         * @implements ILeaderboardEntry
         * @constructor
         * @param {ms.ILeaderboardEntry=} [properties] Properties to set
         */
        function LeaderboardEntry(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * LeaderboardEntry playerId.
         * @member {number} playerId
         * @memberof ms.LeaderboardEntry
         * @instance
         */
        LeaderboardEntry.prototype.playerId = 0;

        /**
         * LeaderboardEntry name.
         * @member {string} name
         * @memberof ms.LeaderboardEntry
         * @instance
         */
        LeaderboardEntry.prototype.name = "";

        /**
         * LeaderboardEntry score.
         * @member {string} score
         * @memberof ms.LeaderboardEntry
         * @instance
         */
        LeaderboardEntry.prototype.score = "";

        /**
         * Creates a new LeaderboardEntry instance using the specified properties.
         * @function create
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {ms.ILeaderboardEntry=} [properties] Properties to set
         * @returns {ms.LeaderboardEntry} LeaderboardEntry instance
         */
        LeaderboardEntry.create = function create(properties) {
            return new LeaderboardEntry(properties);
        };

        /**
         * Encodes the specified LeaderboardEntry message. Does not implicitly {@link ms.LeaderboardEntry.verify|verify} messages.
         * @function encode
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {ms.ILeaderboardEntry} message LeaderboardEntry message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        LeaderboardEntry.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.playerId != null && Object.hasOwnProperty.call(message, "playerId"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.playerId);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
            if (message.score != null && Object.hasOwnProperty.call(message, "score"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.score);
            return writer;
        };

        /**
         * Encodes the specified LeaderboardEntry message, length delimited. Does not implicitly {@link ms.LeaderboardEntry.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {ms.ILeaderboardEntry} message LeaderboardEntry message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        LeaderboardEntry.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a LeaderboardEntry message from the specified reader or buffer.
         * @function decode
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.LeaderboardEntry} LeaderboardEntry
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        LeaderboardEntry.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.LeaderboardEntry();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.playerId = reader.int32();
                        break;
                    }
                case 2: {
                        message.name = reader.string();
                        break;
                    }
                case 3: {
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
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.LeaderboardEntry} LeaderboardEntry
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
         * @memberof ms.LeaderboardEntry
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
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.score != null && message.hasOwnProperty("score"))
                if (!$util.isString(message.score))
                    return "score: string expected";
            return null;
        };

        /**
         * Creates a LeaderboardEntry message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.LeaderboardEntry} LeaderboardEntry
         */
        LeaderboardEntry.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.LeaderboardEntry)
                return object;
            let message = new $root.ms.LeaderboardEntry();
            if (object.playerId != null)
                message.playerId = object.playerId | 0;
            if (object.name != null)
                message.name = String(object.name);
            if (object.score != null)
                message.score = String(object.score);
            return message;
        };

        /**
         * Creates a plain object from a LeaderboardEntry message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {ms.LeaderboardEntry} message LeaderboardEntry
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        LeaderboardEntry.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.playerId = 0;
                object.name = "";
                object.score = "";
            }
            if (message.playerId != null && message.hasOwnProperty("playerId"))
                object.playerId = message.playerId;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.score != null && message.hasOwnProperty("score"))
                object.score = message.score;
            return object;
        };

        /**
         * Converts this LeaderboardEntry to JSON.
         * @function toJSON
         * @memberof ms.LeaderboardEntry
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        LeaderboardEntry.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for LeaderboardEntry
         * @function getTypeUrl
         * @memberof ms.LeaderboardEntry
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        LeaderboardEntry.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.LeaderboardEntry";
        };

        return LeaderboardEntry;
    })();

    ms.Leaderboard = (function() {

        /**
         * Properties of a Leaderboard.
         * @memberof ms
         * @interface ILeaderboard
         * @property {number|Long|null} [version] Leaderboard version
         * @property {Array.<ms.ILeaderboardEntry>|null} [entries] Leaderboard entries
         */

        /**
         * Constructs a new Leaderboard.
         * @memberof ms
         * @classdesc Represents a Leaderboard.
         * @implements ILeaderboard
         * @constructor
         * @param {ms.ILeaderboard=} [properties] Properties to set
         */
        function Leaderboard(properties) {
            this.entries = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Leaderboard version.
         * @member {number|Long} version
         * @memberof ms.Leaderboard
         * @instance
         */
        Leaderboard.prototype.version = $util.Long ? $util.Long.fromBits(0,0,true) : 0;

        /**
         * Leaderboard entries.
         * @member {Array.<ms.ILeaderboardEntry>} entries
         * @memberof ms.Leaderboard
         * @instance
         */
        Leaderboard.prototype.entries = $util.emptyArray;

        /**
         * Creates a new Leaderboard instance using the specified properties.
         * @function create
         * @memberof ms.Leaderboard
         * @static
         * @param {ms.ILeaderboard=} [properties] Properties to set
         * @returns {ms.Leaderboard} Leaderboard instance
         */
        Leaderboard.create = function create(properties) {
            return new Leaderboard(properties);
        };

        /**
         * Encodes the specified Leaderboard message. Does not implicitly {@link ms.Leaderboard.verify|verify} messages.
         * @function encode
         * @memberof ms.Leaderboard
         * @static
         * @param {ms.ILeaderboard} message Leaderboard message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Leaderboard.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.version != null && Object.hasOwnProperty.call(message, "version"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint64(message.version);
            if (message.entries != null && message.entries.length)
                for (let i = 0; i < message.entries.length; ++i)
                    $root.ms.LeaderboardEntry.encode(message.entries[i], writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified Leaderboard message, length delimited. Does not implicitly {@link ms.Leaderboard.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Leaderboard
         * @static
         * @param {ms.ILeaderboard} message Leaderboard message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Leaderboard.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Leaderboard message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Leaderboard
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Leaderboard} Leaderboard
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Leaderboard.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Leaderboard();
            while (reader.pos < end) {
                let tag = reader.uint32();
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
                        message.entries.push($root.ms.LeaderboardEntry.decode(reader, reader.uint32()));
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
         * @memberof ms.Leaderboard
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Leaderboard} Leaderboard
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
         * @memberof ms.Leaderboard
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
                for (let i = 0; i < message.entries.length; ++i) {
                    let error = $root.ms.LeaderboardEntry.verify(message.entries[i]);
                    if (error)
                        return "entries." + error;
                }
            }
            return null;
        };

        /**
         * Creates a Leaderboard message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Leaderboard
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Leaderboard} Leaderboard
         */
        Leaderboard.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Leaderboard)
                return object;
            let message = new $root.ms.Leaderboard();
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
                    throw TypeError(".ms.Leaderboard.entries: array expected");
                message.entries = [];
                for (let i = 0; i < object.entries.length; ++i) {
                    if (typeof object.entries[i] !== "object")
                        throw TypeError(".ms.Leaderboard.entries: object expected");
                    message.entries[i] = $root.ms.LeaderboardEntry.fromObject(object.entries[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a Leaderboard message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Leaderboard
         * @static
         * @param {ms.Leaderboard} message Leaderboard
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Leaderboard.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.entries = [];
            if (options.defaults)
                if ($util.Long) {
                    let long = new $util.Long(0, 0, true);
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
                for (let j = 0; j < message.entries.length; ++j)
                    object.entries[j] = $root.ms.LeaderboardEntry.toObject(message.entries[j], options);
            }
            return object;
        };

        /**
         * Converts this Leaderboard to JSON.
         * @function toJSON
         * @memberof ms.Leaderboard
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Leaderboard.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Leaderboard
         * @function getTypeUrl
         * @memberof ms.Leaderboard
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Leaderboard.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Leaderboard";
        };

        return Leaderboard;
    })();

    ms.ScoreUpdate = (function() {

        /**
         * Properties of a ScoreUpdate.
         * @memberof ms
         * @interface IScoreUpdate
         * @property {number|null} [score] ScoreUpdate score
         * @property {number|null} [worldX] ScoreUpdate worldX
         * @property {number|null} [worldY] ScoreUpdate worldY
         * @property {number|null} [delta] ScoreUpdate delta
         */

        /**
         * Constructs a new ScoreUpdate.
         * @memberof ms
         * @classdesc Represents a ScoreUpdate.
         * @implements IScoreUpdate
         * @constructor
         * @param {ms.IScoreUpdate=} [properties] Properties to set
         */
        function ScoreUpdate(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ScoreUpdate score.
         * @member {number} score
         * @memberof ms.ScoreUpdate
         * @instance
         */
        ScoreUpdate.prototype.score = 0;

        /**
         * ScoreUpdate worldX.
         * @member {number} worldX
         * @memberof ms.ScoreUpdate
         * @instance
         */
        ScoreUpdate.prototype.worldX = 0;

        /**
         * ScoreUpdate worldY.
         * @member {number} worldY
         * @memberof ms.ScoreUpdate
         * @instance
         */
        ScoreUpdate.prototype.worldY = 0;

        /**
         * ScoreUpdate delta.
         * @member {number} delta
         * @memberof ms.ScoreUpdate
         * @instance
         */
        ScoreUpdate.prototype.delta = 0;

        /**
         * Creates a new ScoreUpdate instance using the specified properties.
         * @function create
         * @memberof ms.ScoreUpdate
         * @static
         * @param {ms.IScoreUpdate=} [properties] Properties to set
         * @returns {ms.ScoreUpdate} ScoreUpdate instance
         */
        ScoreUpdate.create = function create(properties) {
            return new ScoreUpdate(properties);
        };

        /**
         * Encodes the specified ScoreUpdate message. Does not implicitly {@link ms.ScoreUpdate.verify|verify} messages.
         * @function encode
         * @memberof ms.ScoreUpdate
         * @static
         * @param {ms.IScoreUpdate} message ScoreUpdate message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ScoreUpdate.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.score != null && Object.hasOwnProperty.call(message, "score"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.score);
            if (message.worldX != null && Object.hasOwnProperty.call(message, "worldX"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.worldX);
            if (message.worldY != null && Object.hasOwnProperty.call(message, "worldY"))
                writer.uint32(/* id 3, wireType 0 =*/24).int32(message.worldY);
            if (message.delta != null && Object.hasOwnProperty.call(message, "delta"))
                writer.uint32(/* id 4, wireType 0 =*/32).int32(message.delta);
            return writer;
        };

        /**
         * Encodes the specified ScoreUpdate message, length delimited. Does not implicitly {@link ms.ScoreUpdate.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.ScoreUpdate
         * @static
         * @param {ms.IScoreUpdate} message ScoreUpdate message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ScoreUpdate.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ScoreUpdate message from the specified reader or buffer.
         * @function decode
         * @memberof ms.ScoreUpdate
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.ScoreUpdate} ScoreUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ScoreUpdate.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.ScoreUpdate();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.score = reader.int32();
                        break;
                    }
                case 2: {
                        message.worldX = reader.int32();
                        break;
                    }
                case 3: {
                        message.worldY = reader.int32();
                        break;
                    }
                case 4: {
                        message.delta = reader.int32();
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
         * Decodes a ScoreUpdate message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.ScoreUpdate
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.ScoreUpdate} ScoreUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ScoreUpdate.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ScoreUpdate message.
         * @function verify
         * @memberof ms.ScoreUpdate
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ScoreUpdate.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.score != null && message.hasOwnProperty("score"))
                if (!$util.isInteger(message.score))
                    return "score: integer expected";
            if (message.worldX != null && message.hasOwnProperty("worldX"))
                if (!$util.isInteger(message.worldX))
                    return "worldX: integer expected";
            if (message.worldY != null && message.hasOwnProperty("worldY"))
                if (!$util.isInteger(message.worldY))
                    return "worldY: integer expected";
            if (message.delta != null && message.hasOwnProperty("delta"))
                if (!$util.isInteger(message.delta))
                    return "delta: integer expected";
            return null;
        };

        /**
         * Creates a ScoreUpdate message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.ScoreUpdate
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.ScoreUpdate} ScoreUpdate
         */
        ScoreUpdate.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.ScoreUpdate)
                return object;
            let message = new $root.ms.ScoreUpdate();
            if (object.score != null)
                message.score = object.score | 0;
            if (object.worldX != null)
                message.worldX = object.worldX | 0;
            if (object.worldY != null)
                message.worldY = object.worldY | 0;
            if (object.delta != null)
                message.delta = object.delta | 0;
            return message;
        };

        /**
         * Creates a plain object from a ScoreUpdate message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.ScoreUpdate
         * @static
         * @param {ms.ScoreUpdate} message ScoreUpdate
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ScoreUpdate.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.score = 0;
                object.worldX = 0;
                object.worldY = 0;
                object.delta = 0;
            }
            if (message.score != null && message.hasOwnProperty("score"))
                object.score = message.score;
            if (message.worldX != null && message.hasOwnProperty("worldX"))
                object.worldX = message.worldX;
            if (message.worldY != null && message.hasOwnProperty("worldY"))
                object.worldY = message.worldY;
            if (message.delta != null && message.hasOwnProperty("delta"))
                object.delta = message.delta;
            return object;
        };

        /**
         * Converts this ScoreUpdate to JSON.
         * @function toJSON
         * @memberof ms.ScoreUpdate
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ScoreUpdate.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ScoreUpdate
         * @function getTypeUrl
         * @memberof ms.ScoreUpdate
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ScoreUpdate.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.ScoreUpdate";
        };

        return ScoreUpdate;
    })();

    ms.ViewUpdate = (function() {

        /**
         * Properties of a ViewUpdate.
         * @memberof ms
         * @interface IViewUpdate
         * @property {number|null} [viewX] ViewUpdate viewX
         * @property {number|null} [viewY] ViewUpdate viewY
         */

        /**
         * Constructs a new ViewUpdate.
         * @memberof ms
         * @classdesc Represents a ViewUpdate.
         * @implements IViewUpdate
         * @constructor
         * @param {ms.IViewUpdate=} [properties] Properties to set
         */
        function ViewUpdate(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ViewUpdate viewX.
         * @member {number} viewX
         * @memberof ms.ViewUpdate
         * @instance
         */
        ViewUpdate.prototype.viewX = 0;

        /**
         * ViewUpdate viewY.
         * @member {number} viewY
         * @memberof ms.ViewUpdate
         * @instance
         */
        ViewUpdate.prototype.viewY = 0;

        /**
         * Creates a new ViewUpdate instance using the specified properties.
         * @function create
         * @memberof ms.ViewUpdate
         * @static
         * @param {ms.IViewUpdate=} [properties] Properties to set
         * @returns {ms.ViewUpdate} ViewUpdate instance
         */
        ViewUpdate.create = function create(properties) {
            return new ViewUpdate(properties);
        };

        /**
         * Encodes the specified ViewUpdate message. Does not implicitly {@link ms.ViewUpdate.verify|verify} messages.
         * @function encode
         * @memberof ms.ViewUpdate
         * @static
         * @param {ms.IViewUpdate} message ViewUpdate message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ViewUpdate.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.viewX != null && Object.hasOwnProperty.call(message, "viewX"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.viewX);
            if (message.viewY != null && Object.hasOwnProperty.call(message, "viewY"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.viewY);
            return writer;
        };

        /**
         * Encodes the specified ViewUpdate message, length delimited. Does not implicitly {@link ms.ViewUpdate.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.ViewUpdate
         * @static
         * @param {ms.IViewUpdate} message ViewUpdate message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ViewUpdate.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ViewUpdate message from the specified reader or buffer.
         * @function decode
         * @memberof ms.ViewUpdate
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.ViewUpdate} ViewUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ViewUpdate.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.ViewUpdate();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.viewX = reader.int32();
                        break;
                    }
                case 2: {
                        message.viewY = reader.int32();
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
         * Decodes a ViewUpdate message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.ViewUpdate
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.ViewUpdate} ViewUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ViewUpdate.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ViewUpdate message.
         * @function verify
         * @memberof ms.ViewUpdate
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ViewUpdate.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.viewX != null && message.hasOwnProperty("viewX"))
                if (!$util.isInteger(message.viewX))
                    return "viewX: integer expected";
            if (message.viewY != null && message.hasOwnProperty("viewY"))
                if (!$util.isInteger(message.viewY))
                    return "viewY: integer expected";
            return null;
        };

        /**
         * Creates a ViewUpdate message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.ViewUpdate
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.ViewUpdate} ViewUpdate
         */
        ViewUpdate.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.ViewUpdate)
                return object;
            let message = new $root.ms.ViewUpdate();
            if (object.viewX != null)
                message.viewX = object.viewX | 0;
            if (object.viewY != null)
                message.viewY = object.viewY | 0;
            return message;
        };

        /**
         * Creates a plain object from a ViewUpdate message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.ViewUpdate
         * @static
         * @param {ms.ViewUpdate} message ViewUpdate
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ViewUpdate.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.viewX = 0;
                object.viewY = 0;
            }
            if (message.viewX != null && message.hasOwnProperty("viewX"))
                object.viewX = message.viewX;
            if (message.viewY != null && message.hasOwnProperty("viewY"))
                object.viewY = message.viewY;
            return object;
        };

        /**
         * Converts this ViewUpdate to JSON.
         * @function toJSON
         * @memberof ms.ViewUpdate
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ViewUpdate.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ViewUpdate
         * @function getTypeUrl
         * @memberof ms.ViewUpdate
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ViewUpdate.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.ViewUpdate";
        };

        return ViewUpdate;
    })();

    ms.Msg = (function() {

        /**
         * Properties of a Msg.
         * @memberof ms
         * @interface IMsg
         * @property {ms.IHello|null} [hello] Msg hello
         * @property {ms.IWelcome|null} [welcome] Msg welcome
         * @property {ms.IReveal|null} [reveal] Msg reveal
         * @property {ms.IFlag|null} [flag] Msg flag
         * @property {ms.ISubscribe|null} [subscribe] Msg subscribe
         * @property {ms.IUnsubscribe|null} [unsubscribe] Msg unsubscribe
         * @property {ms.IChunkSync|null} [chunkSync] Msg chunkSync
         * @property {ms.IRevealAck|null} [revealAck] Msg revealAck
         * @property {ms.IFlagAck|null} [flagAck] Msg flagAck
         * @property {ms.ILeaderboard|null} [leaderboard] Msg leaderboard
         * @property {ms.IScoreUpdate|null} [scoreUpdate] Msg scoreUpdate
         * @property {ms.IViewUpdate|null} [viewUpdate] Msg viewUpdate
         */

        /**
         * Constructs a new Msg.
         * @memberof ms
         * @classdesc Represents a Msg.
         * @implements IMsg
         * @constructor
         * @param {ms.IMsg=} [properties] Properties to set
         */
        function Msg(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Msg hello.
         * @member {ms.IHello|null|undefined} hello
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.hello = null;

        /**
         * Msg welcome.
         * @member {ms.IWelcome|null|undefined} welcome
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.welcome = null;

        /**
         * Msg reveal.
         * @member {ms.IReveal|null|undefined} reveal
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.reveal = null;

        /**
         * Msg flag.
         * @member {ms.IFlag|null|undefined} flag
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.flag = null;

        /**
         * Msg subscribe.
         * @member {ms.ISubscribe|null|undefined} subscribe
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.subscribe = null;

        /**
         * Msg unsubscribe.
         * @member {ms.IUnsubscribe|null|undefined} unsubscribe
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.unsubscribe = null;

        /**
         * Msg chunkSync.
         * @member {ms.IChunkSync|null|undefined} chunkSync
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.chunkSync = null;

        /**
         * Msg revealAck.
         * @member {ms.IRevealAck|null|undefined} revealAck
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.revealAck = null;

        /**
         * Msg flagAck.
         * @member {ms.IFlagAck|null|undefined} flagAck
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.flagAck = null;

        /**
         * Msg leaderboard.
         * @member {ms.ILeaderboard|null|undefined} leaderboard
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.leaderboard = null;

        /**
         * Msg scoreUpdate.
         * @member {ms.IScoreUpdate|null|undefined} scoreUpdate
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.scoreUpdate = null;

        /**
         * Msg viewUpdate.
         * @member {ms.IViewUpdate|null|undefined} viewUpdate
         * @memberof ms.Msg
         * @instance
         */
        Msg.prototype.viewUpdate = null;

        // OneOf field names bound to virtual getters and setters
        let $oneOfFields;

        /**
         * Msg payload.
         * @member {"hello"|"welcome"|"reveal"|"flag"|"subscribe"|"unsubscribe"|"chunkSync"|"revealAck"|"flagAck"|"leaderboard"|"scoreUpdate"|"viewUpdate"|undefined} payload
         * @memberof ms.Msg
         * @instance
         */
        Object.defineProperty(Msg.prototype, "payload", {
            get: $util.oneOfGetter($oneOfFields = ["hello", "welcome", "reveal", "flag", "subscribe", "unsubscribe", "chunkSync", "revealAck", "flagAck", "leaderboard", "scoreUpdate", "viewUpdate"]),
            set: $util.oneOfSetter($oneOfFields)
        });

        /**
         * Creates a new Msg instance using the specified properties.
         * @function create
         * @memberof ms.Msg
         * @static
         * @param {ms.IMsg=} [properties] Properties to set
         * @returns {ms.Msg} Msg instance
         */
        Msg.create = function create(properties) {
            return new Msg(properties);
        };

        /**
         * Encodes the specified Msg message. Does not implicitly {@link ms.Msg.verify|verify} messages.
         * @function encode
         * @memberof ms.Msg
         * @static
         * @param {ms.IMsg} message Msg message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Msg.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.hello != null && Object.hasOwnProperty.call(message, "hello"))
                $root.ms.Hello.encode(message.hello, writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            if (message.welcome != null && Object.hasOwnProperty.call(message, "welcome"))
                $root.ms.Welcome.encode(message.welcome, writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
            if (message.reveal != null && Object.hasOwnProperty.call(message, "reveal"))
                $root.ms.Reveal.encode(message.reveal, writer.uint32(/* id 3, wireType 2 =*/26).fork()).ldelim();
            if (message.flag != null && Object.hasOwnProperty.call(message, "flag"))
                $root.ms.Flag.encode(message.flag, writer.uint32(/* id 4, wireType 2 =*/34).fork()).ldelim();
            if (message.subscribe != null && Object.hasOwnProperty.call(message, "subscribe"))
                $root.ms.Subscribe.encode(message.subscribe, writer.uint32(/* id 5, wireType 2 =*/42).fork()).ldelim();
            if (message.unsubscribe != null && Object.hasOwnProperty.call(message, "unsubscribe"))
                $root.ms.Unsubscribe.encode(message.unsubscribe, writer.uint32(/* id 6, wireType 2 =*/50).fork()).ldelim();
            if (message.chunkSync != null && Object.hasOwnProperty.call(message, "chunkSync"))
                $root.ms.ChunkSync.encode(message.chunkSync, writer.uint32(/* id 7, wireType 2 =*/58).fork()).ldelim();
            if (message.revealAck != null && Object.hasOwnProperty.call(message, "revealAck"))
                $root.ms.RevealAck.encode(message.revealAck, writer.uint32(/* id 8, wireType 2 =*/66).fork()).ldelim();
            if (message.flagAck != null && Object.hasOwnProperty.call(message, "flagAck"))
                $root.ms.FlagAck.encode(message.flagAck, writer.uint32(/* id 9, wireType 2 =*/74).fork()).ldelim();
            if (message.leaderboard != null && Object.hasOwnProperty.call(message, "leaderboard"))
                $root.ms.Leaderboard.encode(message.leaderboard, writer.uint32(/* id 10, wireType 2 =*/82).fork()).ldelim();
            if (message.scoreUpdate != null && Object.hasOwnProperty.call(message, "scoreUpdate"))
                $root.ms.ScoreUpdate.encode(message.scoreUpdate, writer.uint32(/* id 11, wireType 2 =*/90).fork()).ldelim();
            if (message.viewUpdate != null && Object.hasOwnProperty.call(message, "viewUpdate"))
                $root.ms.ViewUpdate.encode(message.viewUpdate, writer.uint32(/* id 12, wireType 2 =*/98).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified Msg message, length delimited. Does not implicitly {@link ms.Msg.verify|verify} messages.
         * @function encodeDelimited
         * @memberof ms.Msg
         * @static
         * @param {ms.IMsg} message Msg message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Msg.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Msg message from the specified reader or buffer.
         * @function decode
         * @memberof ms.Msg
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {ms.Msg} Msg
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Msg.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.ms.Msg();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.hello = $root.ms.Hello.decode(reader, reader.uint32());
                        break;
                    }
                case 2: {
                        message.welcome = $root.ms.Welcome.decode(reader, reader.uint32());
                        break;
                    }
                case 3: {
                        message.reveal = $root.ms.Reveal.decode(reader, reader.uint32());
                        break;
                    }
                case 4: {
                        message.flag = $root.ms.Flag.decode(reader, reader.uint32());
                        break;
                    }
                case 5: {
                        message.subscribe = $root.ms.Subscribe.decode(reader, reader.uint32());
                        break;
                    }
                case 6: {
                        message.unsubscribe = $root.ms.Unsubscribe.decode(reader, reader.uint32());
                        break;
                    }
                case 7: {
                        message.chunkSync = $root.ms.ChunkSync.decode(reader, reader.uint32());
                        break;
                    }
                case 8: {
                        message.revealAck = $root.ms.RevealAck.decode(reader, reader.uint32());
                        break;
                    }
                case 9: {
                        message.flagAck = $root.ms.FlagAck.decode(reader, reader.uint32());
                        break;
                    }
                case 10: {
                        message.leaderboard = $root.ms.Leaderboard.decode(reader, reader.uint32());
                        break;
                    }
                case 11: {
                        message.scoreUpdate = $root.ms.ScoreUpdate.decode(reader, reader.uint32());
                        break;
                    }
                case 12: {
                        message.viewUpdate = $root.ms.ViewUpdate.decode(reader, reader.uint32());
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
         * Decodes a Msg message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof ms.Msg
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {ms.Msg} Msg
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Msg.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Msg message.
         * @function verify
         * @memberof ms.Msg
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Msg.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            let properties = {};
            if (message.hello != null && message.hasOwnProperty("hello")) {
                properties.payload = 1;
                {
                    let error = $root.ms.Hello.verify(message.hello);
                    if (error)
                        return "hello." + error;
                }
            }
            if (message.welcome != null && message.hasOwnProperty("welcome")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.Welcome.verify(message.welcome);
                    if (error)
                        return "welcome." + error;
                }
            }
            if (message.reveal != null && message.hasOwnProperty("reveal")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.Reveal.verify(message.reveal);
                    if (error)
                        return "reveal." + error;
                }
            }
            if (message.flag != null && message.hasOwnProperty("flag")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.Flag.verify(message.flag);
                    if (error)
                        return "flag." + error;
                }
            }
            if (message.subscribe != null && message.hasOwnProperty("subscribe")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.Subscribe.verify(message.subscribe);
                    if (error)
                        return "subscribe." + error;
                }
            }
            if (message.unsubscribe != null && message.hasOwnProperty("unsubscribe")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.Unsubscribe.verify(message.unsubscribe);
                    if (error)
                        return "unsubscribe." + error;
                }
            }
            if (message.chunkSync != null && message.hasOwnProperty("chunkSync")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.ChunkSync.verify(message.chunkSync);
                    if (error)
                        return "chunkSync." + error;
                }
            }
            if (message.revealAck != null && message.hasOwnProperty("revealAck")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.RevealAck.verify(message.revealAck);
                    if (error)
                        return "revealAck." + error;
                }
            }
            if (message.flagAck != null && message.hasOwnProperty("flagAck")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.FlagAck.verify(message.flagAck);
                    if (error)
                        return "flagAck." + error;
                }
            }
            if (message.leaderboard != null && message.hasOwnProperty("leaderboard")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.Leaderboard.verify(message.leaderboard);
                    if (error)
                        return "leaderboard." + error;
                }
            }
            if (message.scoreUpdate != null && message.hasOwnProperty("scoreUpdate")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.ScoreUpdate.verify(message.scoreUpdate);
                    if (error)
                        return "scoreUpdate." + error;
                }
            }
            if (message.viewUpdate != null && message.hasOwnProperty("viewUpdate")) {
                if (properties.payload === 1)
                    return "payload: multiple values";
                properties.payload = 1;
                {
                    let error = $root.ms.ViewUpdate.verify(message.viewUpdate);
                    if (error)
                        return "viewUpdate." + error;
                }
            }
            return null;
        };

        /**
         * Creates a Msg message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof ms.Msg
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {ms.Msg} Msg
         */
        Msg.fromObject = function fromObject(object) {
            if (object instanceof $root.ms.Msg)
                return object;
            let message = new $root.ms.Msg();
            if (object.hello != null) {
                if (typeof object.hello !== "object")
                    throw TypeError(".ms.Msg.hello: object expected");
                message.hello = $root.ms.Hello.fromObject(object.hello);
            }
            if (object.welcome != null) {
                if (typeof object.welcome !== "object")
                    throw TypeError(".ms.Msg.welcome: object expected");
                message.welcome = $root.ms.Welcome.fromObject(object.welcome);
            }
            if (object.reveal != null) {
                if (typeof object.reveal !== "object")
                    throw TypeError(".ms.Msg.reveal: object expected");
                message.reveal = $root.ms.Reveal.fromObject(object.reveal);
            }
            if (object.flag != null) {
                if (typeof object.flag !== "object")
                    throw TypeError(".ms.Msg.flag: object expected");
                message.flag = $root.ms.Flag.fromObject(object.flag);
            }
            if (object.subscribe != null) {
                if (typeof object.subscribe !== "object")
                    throw TypeError(".ms.Msg.subscribe: object expected");
                message.subscribe = $root.ms.Subscribe.fromObject(object.subscribe);
            }
            if (object.unsubscribe != null) {
                if (typeof object.unsubscribe !== "object")
                    throw TypeError(".ms.Msg.unsubscribe: object expected");
                message.unsubscribe = $root.ms.Unsubscribe.fromObject(object.unsubscribe);
            }
            if (object.chunkSync != null) {
                if (typeof object.chunkSync !== "object")
                    throw TypeError(".ms.Msg.chunkSync: object expected");
                message.chunkSync = $root.ms.ChunkSync.fromObject(object.chunkSync);
            }
            if (object.revealAck != null) {
                if (typeof object.revealAck !== "object")
                    throw TypeError(".ms.Msg.revealAck: object expected");
                message.revealAck = $root.ms.RevealAck.fromObject(object.revealAck);
            }
            if (object.flagAck != null) {
                if (typeof object.flagAck !== "object")
                    throw TypeError(".ms.Msg.flagAck: object expected");
                message.flagAck = $root.ms.FlagAck.fromObject(object.flagAck);
            }
            if (object.leaderboard != null) {
                if (typeof object.leaderboard !== "object")
                    throw TypeError(".ms.Msg.leaderboard: object expected");
                message.leaderboard = $root.ms.Leaderboard.fromObject(object.leaderboard);
            }
            if (object.scoreUpdate != null) {
                if (typeof object.scoreUpdate !== "object")
                    throw TypeError(".ms.Msg.scoreUpdate: object expected");
                message.scoreUpdate = $root.ms.ScoreUpdate.fromObject(object.scoreUpdate);
            }
            if (object.viewUpdate != null) {
                if (typeof object.viewUpdate !== "object")
                    throw TypeError(".ms.Msg.viewUpdate: object expected");
                message.viewUpdate = $root.ms.ViewUpdate.fromObject(object.viewUpdate);
            }
            return message;
        };

        /**
         * Creates a plain object from a Msg message. Also converts values to other types if specified.
         * @function toObject
         * @memberof ms.Msg
         * @static
         * @param {ms.Msg} message Msg
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Msg.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (message.hello != null && message.hasOwnProperty("hello")) {
                object.hello = $root.ms.Hello.toObject(message.hello, options);
                if (options.oneofs)
                    object.payload = "hello";
            }
            if (message.welcome != null && message.hasOwnProperty("welcome")) {
                object.welcome = $root.ms.Welcome.toObject(message.welcome, options);
                if (options.oneofs)
                    object.payload = "welcome";
            }
            if (message.reveal != null && message.hasOwnProperty("reveal")) {
                object.reveal = $root.ms.Reveal.toObject(message.reveal, options);
                if (options.oneofs)
                    object.payload = "reveal";
            }
            if (message.flag != null && message.hasOwnProperty("flag")) {
                object.flag = $root.ms.Flag.toObject(message.flag, options);
                if (options.oneofs)
                    object.payload = "flag";
            }
            if (message.subscribe != null && message.hasOwnProperty("subscribe")) {
                object.subscribe = $root.ms.Subscribe.toObject(message.subscribe, options);
                if (options.oneofs)
                    object.payload = "subscribe";
            }
            if (message.unsubscribe != null && message.hasOwnProperty("unsubscribe")) {
                object.unsubscribe = $root.ms.Unsubscribe.toObject(message.unsubscribe, options);
                if (options.oneofs)
                    object.payload = "unsubscribe";
            }
            if (message.chunkSync != null && message.hasOwnProperty("chunkSync")) {
                object.chunkSync = $root.ms.ChunkSync.toObject(message.chunkSync, options);
                if (options.oneofs)
                    object.payload = "chunkSync";
            }
            if (message.revealAck != null && message.hasOwnProperty("revealAck")) {
                object.revealAck = $root.ms.RevealAck.toObject(message.revealAck, options);
                if (options.oneofs)
                    object.payload = "revealAck";
            }
            if (message.flagAck != null && message.hasOwnProperty("flagAck")) {
                object.flagAck = $root.ms.FlagAck.toObject(message.flagAck, options);
                if (options.oneofs)
                    object.payload = "flagAck";
            }
            if (message.leaderboard != null && message.hasOwnProperty("leaderboard")) {
                object.leaderboard = $root.ms.Leaderboard.toObject(message.leaderboard, options);
                if (options.oneofs)
                    object.payload = "leaderboard";
            }
            if (message.scoreUpdate != null && message.hasOwnProperty("scoreUpdate")) {
                object.scoreUpdate = $root.ms.ScoreUpdate.toObject(message.scoreUpdate, options);
                if (options.oneofs)
                    object.payload = "scoreUpdate";
            }
            if (message.viewUpdate != null && message.hasOwnProperty("viewUpdate")) {
                object.viewUpdate = $root.ms.ViewUpdate.toObject(message.viewUpdate, options);
                if (options.oneofs)
                    object.payload = "viewUpdate";
            }
            return object;
        };

        /**
         * Converts this Msg to JSON.
         * @function toJSON
         * @memberof ms.Msg
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Msg.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Msg
         * @function getTypeUrl
         * @memberof ms.Msg
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Msg.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/ms.Msg";
        };

        return Msg;
    })();

    return ms;
})();

export { $root as default };
