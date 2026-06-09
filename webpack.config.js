const path = require('path');
const webpack = require('webpack');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const CopyWebpackPlugin = require('copy-webpack-plugin');

class EntrypointsPlugin {
    apply(compiler) {
        compiler.hooks.thisCompilation.tap('EntrypointsPlugin', (compilation) => {
            compilation.hooks.processAssets.tap(
                {
                    name: 'EntrypointsPlugin',
                    stage: webpack.Compilation.PROCESS_ASSETS_STAGE_REPORT,
                },
                () => {
                    const entrypoints = {};

                    for (const [name, entrypoint] of compilation.entrypoints) {
                        const files = entrypoint.getFiles()
                            .filter((file) => !file.endsWith('.map'));

                        entrypoints[name] = {
                            css: files.filter((file) => file.endsWith('.css')).map((file) => `/build/${file}`),
                            js: files.filter((file) => file.endsWith('.js')).map((file) => `/build/${file}`),
                        };
                    }

                    compilation.emitAsset(
                        'entrypoints.json',
                        new webpack.sources.RawSource(JSON.stringify({entrypoints}, null, 2))
                    );
                }
            );
        });
    }
}

module.exports = {
    entry: {
        app: './assets/js/app.js',
        bfd_sessions: './assets/js/bfd_sessions.js',
        bgp_protocols: './assets/js/bgp_protocols.js',
        community_lookup: './assets/js/community_lookup.js',
        network_lookup: './assets/js/network_lookup.js',
        server_routes: './assets/js/server_routes.js',
    },
    output: {
        path: path.resolve(__dirname, 'public/build'),
        publicPath: '/build/',
        filename: '[name].[contenthash:8].js',
        chunkFilename: '[name].[contenthash:8].js',
        clean: true,
    },
    resolve: {
        extensions: ['.js', '.json'],
    },
    module: {
        rules: [
            {
                test: /\.m?js$/,
                exclude: /node_modules/,
                use: {
                    loader: 'babel-loader',
                    options: {
                        presets: [
                            ['@babel/preset-env', {
                                useBuiltIns: 'usage',
                                corejs: 3,
                            }],
                        ],
                    },
                },
            },
            {
                test: /\.(scss|css)$/,
                use: [
                    MiniCssExtractPlugin.loader,
                    {
                        loader: 'css-loader',
                        options: {
                            sourceMap: false,
                        },
                    },
                    'postcss-loader',
                    {
                        loader: 'sass-loader',
                        options: {
                            sassOptions: {
                                quietDeps: true,
                                loadPaths: [path.resolve(__dirname, 'node_modules')],
                            },
                        },
                    },
                ],
            },
            {
                test: /\.(png|jpg|jpeg|gif|svg|woff2?|eot|ttf|otf)$/i,
                type: 'asset/resource',
                generator: {
                    filename: 'assets/[name].[contenthash:8][ext]',
                },
            },
        ],
    },
    optimization: {
        splitChunks: {
            chunks: 'all',
        },
        runtimeChunk: 'single',
    },
    plugins: [
        new MiniCssExtractPlugin({
            filename: '[name].[contenthash:8].css',
        }),
        new CopyWebpackPlugin({
            patterns: [
                {
                    from: 'assets/images',
                    to: 'images/[name][ext]',
                },
            ],
        }),
        new webpack.ProvidePlugin({
            $: 'jquery',
            jQuery: 'jquery',
            'window.jQuery': 'jquery',
        }),
        new webpack.NormalModuleReplacementPlugin(
            /friendsofsymfony\/jsrouting-bundle\/Resources\/public\/js\/router\.min\.js$/,
            path.resolve(__dirname, 'web/frontend-router.js')
        ),
        new EntrypointsPlugin(),
    ],
};
